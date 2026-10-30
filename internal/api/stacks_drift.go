package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/store"
)

// The shared drift token lives APP_STACK_DRIFT_TOKEN_TTL (default 2h) and
// rotates in its last quarter; the TTL bounds a token orphaned by a crash.
const (
	envStackDriftTTL          = "APP_STACK_DRIFT_TOKEN_TTL"
	defaultStackDriftTokenTTL = 2 * time.Hour
	stackDriftTokenName       = "stack-drift"
)

func stackDriftTokenLife() (ttl, renew time.Duration) {
	ttl = envDuration(envStackDriftTTL, defaultStackDriftTokenTTL)
	return ttl, ttl / 4
}

// RevokeStaleStackDriftTokens revokes system-minted drift tokens left by an
// earlier process; call it once at startup, before the first drift pass.
func (rt *Router) RevokeStaleStackDriftTokens(ctx context.Context) {
	toks, err := rt.tokens.ListAPITokens(ctx)
	if err != nil {
		rt.logger.Warn("api: stack drift: list stale tokens failed", slog.String("error", err.Error()))
		return
	}
	for _, t := range toks {
		if t.Name != stackDriftTokenName || t.RevokedAt != nil || t.OwnerUserID != "" || t.AgentName != "" {
			continue
		}
		if err := rt.tokens.RevokeAPIToken(ctx, t.ID); err != nil && !errors.Is(err, store.ErrAPITokenNotFound) {
			rt.logger.Warn("api: stack drift: revoke stale token failed", slog.String("error", err.Error()))
		}
	}
}

// stackDriftReader is the optional store surface behind the drift payload.
type stackDriftReader interface {
	GetStackDrift(ctx context.Context, stackID string) (store.StackDrift, bool, error)
	ListStackDrift(ctx context.Context) ([]store.StackDrift, error)
}

type stackDriftItem struct {
	Kind   string   `json:"kind"`
	Name   string   `json:"name"`
	Scope  string   `json:"scope,omitempty"`
	Action string   `json:"action"`
	Fields []string `json:"fields,omitempty"`
}

// stackDriftResource is the latest drift check; summaries hold field paths
// only, never values.
type stackDriftResource struct {
	Status    string           `json:"status"`
	CheckedAt string           `json:"checked_at,omitempty"`
	Revision  int              `json:"revision,omitempty"`
	Summary   []stackDriftItem `json:"summary"`
}

func stackDriftOf(ctx context.Context, ss StackStore, stackID string) stackDriftResource {
	out := stackDriftResource{Status: store.StackDriftNone, Summary: []stackDriftItem{}}
	rd, ok := ss.(stackDriftReader)
	if !ok {
		return out
	}
	d, found, err := rd.GetStackDrift(ctx, stackID)
	if err != nil || !found {
		return out
	}
	out.Status, out.Revision, out.CheckedAt = d.Status, d.Revision, fmtStackTime(d.CheckedAt)
	for _, it := range d.Summary {
		out.Summary = append(out.Summary, stackDriftItem(it))
	}
	return out
}

// stackDriftToken is the one in-process read token every drift pass shares, so
// the token table does not grow by a row per pass.
type stackDriftToken struct {
	mu      sync.Mutex
	raw     string
	id      string
	expires time.Time
}

// driftToken returns a live token, rotating it (and revoking the old one) when less
// than the renew window of its life is left.
func (rt *Router) driftToken(ctx context.Context) (string, error) {
	d := &rt.stackDrift
	d.mu.Lock()
	defer d.mu.Unlock()
	ttl, renew := stackDriftTokenLife()
	if d.raw != "" && time.Until(d.expires) > renew {
		return d.raw, nil
	}
	if d.id != "" {
		if err := rt.tokens.RevokeAPIToken(context.WithoutCancel(ctx), d.id); err != nil {
			rt.logger.Warn("api: stack drift: revoke read token failed", slog.String("error", err.Error()))
		}
		d.raw, d.id = "", ""
	}
	expires := time.Now().Add(ttl)
	raw, rec, err := MintAgentAPIToken(ctx, rt.tokens, stackDriftTokenName, []string{AbilityRead}, &expires, agentIdentity{}, "")
	if err != nil {
		return "", fmt.Errorf("api: stack drift: mint read token: %w", err)
	}
	d.raw, d.id, d.expires = raw, rec.ID, expires
	return raw, nil
}

// StackDriftLive gives the drift checker read access to live state through
// the full handler chain, on a shared read-only system token.
func (rt *Router) StackDriftLive(ctx context.Context) (iac.Doer, func(), error) {
	raw, err := rt.driftToken(ctx)
	if err != nil {
		return nil, nil, err
	}
	rt.iac.once.Do(func() { rt.iac.h = rt.Handler() })
	orig, err := http.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
	if err != nil {
		return nil, nil, err
	}
	orig.Header.Set("Authorization", "Bearer "+raw)
	orig.RemoteAddr = "127.0.0.1:0"
	return &loopbackDoer{h: rt.iac.h, orig: orig}, func() {}, nil
}

// stackDriftAttentionItems raises one item per drifted stack. The drift policy
// only changes enforcement, never whether the stack is reported.
func (rt *Router) stackDriftAttentionItems(r *http.Request, abilities []string) []attention.Item {
	if !hasAbility(abilities, AbilityRead) {
		return nil
	}
	ss := rt.stackStore()
	rd, ok := ss.(stackDriftReader)
	if !ok {
		return nil
	}
	drifts, err := rd.ListStackDrift(r.Context())
	if err != nil {
		rt.logger.Warn("api: attention feed: list stack drift failed", slog.String("error", err.Error()))
		return nil
	}
	names := map[string]string{}
	if stacks, err := ss.ListStacks(r.Context()); err == nil {
		for _, s := range stacks {
			names[s.ID] = s.Name
		}
	}
	var items []attention.Item
	for _, d := range drifts {
		name, known := names[d.StackID]
		if d.Status != store.StackDriftDrifted || !known {
			continue
		}
		// Read on the feed is not read on this stack: a name alone leaks its existence.
		if !rt.stillAuthorized(r.Context(), r, AbilityRead, "stack:"+name) {
			continue
		}
		detail := fmt.Sprintf("%d resource(s) differ from revision %d, last checked %s", len(d.Summary), d.Revision, d.CheckedAt.UTC().Format(time.RFC3339))
		items = append(items, feedItem(attention.Warning, attention.KindStackDrift, name, detail, map[string]string{"stack": name, "revision": fmt.Sprint(d.Revision)}))
	}
	return items
}

// StackProjectName resolves a project id to its name for the drift checker.
func (rt *Router) StackProjectName(ctx context.Context, id string) string {
	return rt.projectName(ctx, id)
}
