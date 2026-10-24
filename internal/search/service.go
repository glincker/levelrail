package search

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Defaults for the freshness policy.
const (
	DefaultMaxAge   = 60 * time.Second
	dirtyDebounce   = 2 * time.Second
	DefaultPageSize = 20
	MaxPageSize     = 100
	maxUsageRows    = 2000
	maxSavedPerUser = 100
)

// ErrNotFound is returned for a ref the caller may not see or that is gone.
var ErrNotFound = errors.New("search: not found")

// ErrInvalid marks a rejected request.
var ErrInvalid = errors.New("search: invalid request")

var kindWeight = map[Kind]float64{
	KindApp: 1.3, KindDatabase: 1.2, KindAction: 1.15, KindNode: 1.0, KindDomain: 1.0,
	KindProject: 0.95, KindEnv: 0.95, KindSetting: 0.9, KindTemplate: 0.85, KindDeployment: 0.8,
	KindTag: 0.8, KindDoc: 0.75, KindSaved: 1.1,
}

// Service owns the index and answers queries.
type Service struct {
	db        DB
	providers []Provider
	logger    *slog.Logger
	maxAge    time.Duration
	now       func() time.Time

	mu         sync.Mutex
	lastRun    time.Time
	dirtySince atomic.Int64
	// indexGen changes whenever search_docs is written, which drops cached candidates.
	indexGen atomic.Uint64
	cache    candCache
}

// New builds a Service over db and providers.
func New(db DB, logger *slog.Logger, providers ...Provider) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{db: db, providers: providers, logger: logger, maxAge: DefaultMaxAge, now: time.Now}
}

// Providers lists the registered providers.
func (s *Service) Providers() []Provider { return s.providers }

// MarkDirty records that a store changed; the next query reconciles.
func (s *Service) MarkDirty() { s.dirtySince.CompareAndSwap(0, s.now().UnixNano()) }

// Reconcile re-reads every provider into the index.
func (s *Service) Reconcile(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reconcileLocked(ctx, nil)
}

// Reindex re-reads one kind, the hook a store change calls.
func (s *Service) Reindex(ctx context.Context, kind Kind) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reconcileLocked(ctx, &kind)
}

func (s *Service) reconcileLocked(ctx context.Context, only *Kind) error {
	var errs []error
	for _, p := range s.providers {
		if only != nil && p.Kind() != *only {
			continue
		}
		docs, err := p.Docs(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("search: provider %s: %w", p.Kind(), err))
			continue
		}
		err = replaceKind(ctx, s.db, p.Kind(), docs)
		s.indexGen.Add(1)
		if err != nil {
			errs = append(errs, err)
		}
	}
	if only == nil {
		s.lastRun = s.now()
		s.dirtySince.Store(0)
	}
	return errors.Join(errs...)
}

// ensureFresh reconciles when the index is empty, stale, or dirty past the debounce.
func (s *Service) ensureFresh(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	stale := s.lastRun.IsZero() || now.Sub(s.lastRun) > s.maxAge
	dirty := s.dirtySince.Load() != 0 && now.Sub(s.lastRun) >= dirtyDebounce
	if !stale && !dirty {
		return
	}
	if err := s.reconcileLocked(ctx, nil); err != nil {
		s.logger.Warn("search: reconcile failed", slog.String("error", err.Error()))
	}
}

// Request is one search call.
type Request struct {
	Q      string
	Kind   Kind
	Limit  int
	Cursor string
}

// Response is a page of authorized hits.
type Response struct {
	Hits       []Hit  `json:"hits"`
	NextCursor string `json:"next_cursor,omitempty"`
	Total      int    `json:"total"`
}

// Search runs the query, drops every doc the caller's Authorizer refuses,
// then ranks what is left.
func (s *Service) Search(ctx context.Context, c Caller, req Request) (Response, error) {
	if c.Authz == nil {
		return Response{}, fmt.Errorf("%w: no authorizer", ErrInvalid)
	}
	s.ensureFresh(ctx)
	limit := req.Limit
	if limit <= 0 {
		limit = DefaultPageSize
	}
	limit = min(limit, MaxPageSize)
	offset := 0
	if req.Cursor != "" {
		n, err := strconv.Atoi(req.Cursor)
		if err != nil || n < 0 {
			return Response{}, fmt.Errorf("%w: bad cursor", ErrInvalid)
		}
		offset = n
	}
	hits, terms, err := s.ranked(ctx, c, req)
	if err != nil {
		return Response{}, err
	}
	if hits == nil {
		return Response{Hits: []Hit{}}, nil
	}
	resp := Response{Total: len(hits), Hits: []Hit{}}
	if offset >= len(hits) {
		return resp, nil
	}
	end := min(offset+limit, len(hits))
	for _, h := range hits[offset:end] {
		hit := toHit(h)
		hit.TitleSpans = highlightSpans(h.cd.doc.Title, terms)
		resp.Hits = append(resp.Hits, hit)
	}
	if end < len(hits) {
		resp.NextCursor = strconv.Itoa(end)
	}
	return resp, nil
}

// RankedNames returns the names of every authorized doc of kind matching q in
// rank order, and false when the index has never been built. It filters through
// the same Authorizer as Search.
func (s *Service) RankedNames(ctx context.Context, c Caller, q string, kind Kind) ([]string, bool, error) {
	if c.Authz == nil {
		return nil, false, fmt.Errorf("%w: no authorizer", ErrInvalid)
	}
	s.ensureFresh(ctx)
	s.mu.Lock()
	warm := !s.lastRun.IsZero()
	s.mu.Unlock()
	if !warm {
		return nil, false, nil
	}
	hits, _, err := s.ranked(ctx, c, Request{Q: q, Kind: kind})
	if err != nil {
		return nil, true, err
	}
	names := make([]string, 0, len(hits))
	for _, h := range hits {
		names = append(names, h.cd.doc.Name)
	}
	return names, true, nil
}

// ranked returns every authorized hit in rank order plus the lowercased text
// terms; a nil slice means the query was empty.
func (s *Service) ranked(ctx context.Context, c Caller, req Request) ([]scored, []string, error) {
	q := ParseQuery(req.Q)
	kind := req.Kind
	if q.Kind != "" {
		kind = q.Kind
	}
	if q.Empty() && kind == "" {
		return nil, nil, nil
	}

	var hits []scored
	if kind != KindSaved {
		cands, err := s.candidates(ctx, q, kind)
		if err != nil {
			return nil, nil, err
		}
		for _, cd := range cands {
			if c.Authz.Allowed(cd.doc.Prefix, cd.doc.Name, cd.doc.Ability) {
				hits = append(hits, scored{cd: cd})
			}
		}
	}
	if kind == "" || kind == KindSaved {
		saved, err := s.savedHits(ctx, c.PrincipalID, q)
		if err != nil {
			return nil, nil, err
		}
		hits = append(hits, saved...)
	}

	usage, err := s.usageFor(ctx, c.PrincipalID)
	if err != nil {
		return nil, nil, err
	}
	now := s.now()
	terms := lowerTerms(q.Text)
	for i := range hits {
		hits[i].score = rank(hits[i].cd, terms, usage[hits[i].cd.doc.Ref], now)
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].cd.doc.Ref < hits[j].cd.doc.Ref
	})
	if hits == nil {
		hits = []scored{}
	}

	return hits, terms, nil
}

type scored struct {
	cd    candidate
	score float64
}

func lowerTerms(in []string) []string {
	out := make([]string, len(in))
	for i, t := range in {
		out[i] = strings.ToLower(t)
	}
	return out
}

func rank(cd candidate, terms []string, uses int, now time.Time) float64 {
	d := cd.doc
	w, ok := kindWeight[d.Kind]
	if !ok {
		w = 1
	}
	score := cd.relevance * w
	title := strings.ToLower(d.Title)
	for _, t := range terms {
		switch {
		case title == t:
			score *= 3
		case strings.HasPrefix(title, t):
			score *= 1.5
		}
	}
	if !d.UpdatedAt.IsZero() && d.UpdatedAt.Unix() > 0 {
		ageDays := math.Max(now.Sub(d.UpdatedAt).Hours()/24, 0)
		score *= 1 + 0.3*math.Exp(-ageDays/30)
	}
	if uses > 0 {
		score *= 1 + math.Min(math.Log1p(float64(uses)), 2)*0.25
	}
	return score
}

func toHit(h scored) Hit {
	d := h.cd.doc
	hit := Hit{Ref: d.Ref, Kind: d.Kind, Title: d.Title, Subtitle: trimSubtitle(d.Body), AppName: d.App,
		Env: d.Env, NodeName: d.Node, Tags: d.Tags, Href: d.Href, Score: math.Round(h.score*1000) / 1000}
	if d.Kind == KindAction {
		hit.Action = &ActionInfo{ID: d.ActionID, Ability: d.Ability, Confirm: d.Confirm}
		hit.Action.MCPTool, hit.Action.MCPArgs, hit.Action.MCPNeeds = actionCall(d)
	}
	return hit
}

func trimSubtitle(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 140 {
		return string(r[:140])
	}
	return s
}

func (s *Service) usageFor(ctx context.Context, principal string) (map[string]int, error) {
	out := map[string]int{}
	if principal == "" {
		return out, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT ref, uses FROM search_usage WHERE principal_id = ?`, principal)
	if err != nil {
		return nil, fmt.Errorf("search: usage: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var ref string
		var n int
		if err := rows.Scan(&ref, &n); err != nil {
			return nil, fmt.Errorf("search: scan usage: %w", err)
		}
		out[ref] = n
	}
	return out, rows.Err()
}

// RecordUse bumps the caller's frequency for a hit they opened. A ref the
// caller may not see is reported as not found.
func (s *Service) RecordUse(ctx context.Context, c Caller, ref string) error {
	if c.PrincipalID == "" || c.Authz == nil {
		return fmt.Errorf("%w: no principal", ErrInvalid)
	}
	var d Doc
	var kind string
	err := s.db.QueryRowContext(ctx, `SELECT kind, prefix, name, ability FROM search_docs WHERE ref = ?`, ref).
		Scan(&kind, &d.Prefix, &d.Name, &d.Ability)
	if err != nil || !c.Authz.Allowed(d.Prefix, d.Name, d.Ability) {
		return ErrNotFound
	}
	now := s.now().Unix()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO search_usage (principal_id, ref, uses, last_used_at) VALUES (?, ?, 1, ?)
		ON CONFLICT(principal_id, ref) DO UPDATE SET uses = uses + 1, last_used_at = excluded.last_used_at`,
		c.PrincipalID, ref, now); err != nil {
		return fmt.Errorf("search: record use: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM search_usage WHERE principal_id = ? AND ref NOT IN
		(SELECT ref FROM search_usage WHERE principal_id = ? ORDER BY last_used_at DESC LIMIT ?)`,
		c.PrincipalID, c.PrincipalID, maxUsageRows)
	if err != nil {
		return fmt.Errorf("search: prune usage: %w", err)
	}
	return nil
}

// Suggestion is one autocomplete entry.
type Suggestion struct {
	Text string `json:"text"`
	Kind string `json:"kind"`
	Ref  string `json:"ref,omitempty"`
}

var scopeHints = []string{"app:", "env:", "tag:", "node:", "kind:"}

// Suggest completes scope keys and returns the top authorized titles.
func (s *Service) Suggest(ctx context.Context, c Caller, q string) ([]Suggestion, error) {
	out := []Suggestion{}
	last := q
	if i := strings.LastIndexByte(q, ' '); i >= 0 {
		last = q[i+1:]
	}
	if last != "" && !strings.Contains(last, ":") {
		for _, h := range scopeHints {
			if strings.HasPrefix(h, strings.ToLower(last)) {
				out = append(out, Suggestion{Text: strings.TrimSuffix(q, last) + h, Kind: "scope"})
			}
		}
	}
	resp, err := s.Search(ctx, c, Request{Q: q, Limit: 8})
	if err != nil {
		return nil, err
	}
	for _, h := range resp.Hits {
		out = append(out, Suggestion{Text: h.Title, Kind: string(h.Kind), Ref: h.Ref})
	}
	return out, nil
}

// Saved is a stored query of one principal.
type Saved struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Query     string    `json:"query"`
	CreatedAt time.Time `json:"created_at"`
}

// ListSaved returns the principal's saved searches, newest first.
func (s *Service) ListSaved(ctx context.Context, principal string) ([]Saved, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, query, created_at FROM search_saved WHERE principal_id = ? ORDER BY created_at DESC, name`, principal)
	if err != nil {
		return nil, fmt.Errorf("search: list saved: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []Saved{}
	for rows.Next() {
		var v Saved
		var ts int64
		if err := rows.Scan(&v.ID, &v.Name, &v.Query, &ts); err != nil {
			return nil, fmt.Errorf("search: scan saved: %w", err)
		}
		v.CreatedAt = time.Unix(ts, 0).UTC()
		out = append(out, v)
	}
	return out, rows.Err()
}

// SaveSearch stores or replaces a named query for the principal.
func (s *Service) SaveSearch(ctx context.Context, principal, name, query string) (Saved, error) {
	name, query = strings.TrimSpace(name), strings.TrimSpace(query)
	if principal == "" || name == "" || query == "" || len(name) > 80 || len(query) > 500 {
		return Saved{}, fmt.Errorf("%w: name and query are required (name up to 80, query up to 500 characters)", ErrInvalid)
	}
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM search_saved WHERE principal_id = ? AND name <> ?`, principal, name).Scan(&n); err != nil {
		return Saved{}, fmt.Errorf("search: count saved: %w", err)
	}
	if n >= maxSavedPerUser {
		return Saved{}, fmt.Errorf("%w: at most %d saved searches", ErrInvalid, maxSavedPerUser)
	}
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return Saved{}, fmt.Errorf("search: saved id: %w", err)
	}
	id := "ss_" + hex.EncodeToString(raw[:])
	now := s.now()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO search_saved (id, principal_id, name, query, created_at) VALUES (?,?,?,?,?)
		ON CONFLICT(principal_id, name) DO UPDATE SET query = excluded.query`, id, principal, name, query, now.Unix()); err != nil {
		return Saved{}, fmt.Errorf("search: save: %w", err)
	}
	var out Saved
	var ts int64
	if err := s.db.QueryRowContext(ctx, `SELECT id, name, query, created_at FROM search_saved WHERE principal_id = ? AND name = ?`, principal, name).
		Scan(&out.ID, &out.Name, &out.Query, &ts); err != nil {
		return Saved{}, fmt.Errorf("search: reload saved: %w", err)
	}
	out.CreatedAt = time.Unix(ts, 0).UTC()
	return out, nil
}

// DeleteSaved removes one of the principal's saved searches.
func (s *Service) DeleteSaved(ctx context.Context, principal, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM search_saved WHERE id = ? AND principal_id = ?`, id, principal)
	if err != nil {
		return fmt.Errorf("search: delete saved: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// savedHits surfaces the caller's own saved searches; they are scoped by
// principal in SQL, never by the Authorizer, because they name no resource.
func (s *Service) savedHits(ctx context.Context, principal string, q Query) ([]scored, error) {
	if principal == "" || len(q.Text) == 0 {
		return nil, nil
	}
	all, err := s.ListSaved(ctx, principal)
	if err != nil {
		return nil, err
	}
	var out []scored
	for _, v := range all {
		hay := strings.ToLower(v.Name + " " + v.Query)
		match := true
		for _, t := range q.Text {
			if !strings.Contains(hay, strings.ToLower(t)) {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		out = append(out, scored{cd: candidate{relevance: 1, doc: Doc{
			Ref: "saved:" + v.ID, Kind: KindSaved, Title: v.Name, Body: v.Query, UpdatedAt: v.CreatedAt,
		}}})
	}
	return out, nil
}
