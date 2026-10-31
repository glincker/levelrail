package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/kit/netguard"
	"github.com/GLINCKER/levelrail/kit/semver"
)

// Marketplace errors beyond the index ones. Callers match with errors.Is.
var (
	ErrEntryNotFound = errors.New("plugin: that plugin is not listed in the marketplace source")
	ErrChanged       = errors.New("plugin: the marketplace listing changed since you reviewed it, review it again")
	ErrNoRuntime     = errors.New("plugin: the plugin runtime is not available, so nothing can be installed")
)

// Index error kinds shown next to a Re-check button.
const (
	KindFetch     = "fetch"
	KindSignature = "signature"
	KindFormat    = "format"
	KindReplay    = "replay"
	KindTooLarge  = "too_large"
)

// AuditMarketplaceInstall is written once per one-click install or update.
const AuditMarketplaceInstall = "plugin.marketplace_install"

// BuiltinSourceID names the source configured by the operator's environment.
const BuiltinSourceID = "default"

const (
	indexFetchTimeout = 15 * time.Second
	indexTTL          = 24 * time.Hour
	marketKeySuffix   = " (added from marketplace)"
)

// IndexFetchResult is one conditional GET of an index.
type IndexFetchResult struct {
	Body        []byte
	ETag        string
	NotModified bool
}

// IndexFetchFunc downloads an index from an https URL, sending etag as If-None-Match.
type IndexFetchFunc func(ctx context.Context, rawURL string, limit int64, etag string) (IndexFetchResult, error)

// FetchIndex is the default IndexFetchFunc: https only, SSRF guarded, size capped.
func FetchIndex(ctx context.Context, rawURL string, limit int64, etag string) (IndexFetchResult, error) {
	if err := httpsURL(rawURL); err != nil {
		return IndexFetchResult{}, invalid("the index source must be an https URL")
	}
	c := netguard.NewClient()
	c.Timeout = indexFetchTimeout
	c.CheckRedirect = func(_ *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("too many redirects")
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(rawURL), nil)
	if err != nil {
		return IndexFetchResult{}, invalid("bad index URL: %v", err)
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := c.Do(req) //nolint:gosec // operator-configured URL, sent through the SSRF-guarded client
	if err != nil {
		return IndexFetchResult{}, invalid("fetch index: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotModified && etag != "" {
		return IndexFetchResult{NotModified: true, ETag: etag}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return IndexFetchResult{}, invalid("fetch index: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return IndexFetchResult{}, fmt.Errorf("plugin: read index: %w", err)
	}
	if int64(len(body)) > limit {
		return IndexFetchResult{}, ErrIndexTooLarge
	}
	return IndexFetchResult{Body: body, ETag: resp.Header.Get("ETag")}, nil
}

// Marketplace lists plugins from signed indexes and installs them in one confirmed step.
type Marketplace struct {
	store          MarketStore
	mgr            *Manager
	builtin        *Source
	fetchIndex     IndexFetchFunc
	fetchFile      FetchFunc
	now            func() time.Time
	controlVersion string

	mu sync.Mutex
}

// NewMarketplace builds the service. mgr may be nil, which leaves browsing on
// and installing off. A builtinURL with an unusable key is skipped and reported.
func NewMarketplace(store MarketStore, mgr *Manager, builtinURL, builtinRootKey, controlVersion string) (*Marketplace, error) {
	m := &Marketplace{
		store: store, mgr: mgr, fetchIndex: FetchIndex, fetchFile: Fetch, now: time.Now, controlVersion: controlVersion,
	}
	if strings.TrimSpace(builtinURL) == "" {
		return m, nil
	}
	pub, err := ParsePublicKey(builtinRootKey)
	if err != nil {
		return m, fmt.Errorf("default marketplace source skipped: %w", err)
	}
	if err := httpsURL(builtinURL); err != nil {
		return m, fmt.Errorf("default marketplace source skipped: url %w", err)
	}
	m.builtin = &Source{ID: BuiltinSourceID, Name: "Default", URL: strings.TrimSpace(builtinURL), RootKey: strings.TrimSpace(builtinRootKey), Builtin: true, KeyPrint: Fingerprint(pub)}
	return m, nil
}

// SetFetchers replaces the network fetchers, for tests and offline mirrors.
func (m *Marketplace) SetFetchers(index IndexFetchFunc, file FetchFunc) {
	m.fetchIndex, m.fetchFile = index, file
}

// SourceStatus is a source plus the outcome of its last fetch.
type SourceStatus struct {
	Source
	Status      string `json:"status"`
	Error       string `json:"error"`
	ErrorKind   string `json:"error_kind"`
	Sequence    int64  `json:"sequence"`
	GeneratedAt string `json:"generated_at"`
	FetchedAt   string `json:"fetched_at"`
	PluginCount int    `json:"plugin_count"`
}

// CatalogEntry is an index entry with what the control plane knows about it locally.
type CatalogEntry struct {
	Entry
	SourceID         string `json:"source_id"`
	SourceName       string `json:"source_name"`
	PublisherKeyName string `json:"publisher_key_name"`
	Verified         bool   `json:"verified"`
	KeyTrusted       bool   `json:"key_trusted"`
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installed_version"`
	UpdateAvailable  bool   `json:"update_available"`
	Compatible       bool   `json:"compatible"`
}

// Catalog is every listing from every source plus each source's status.
type Catalog struct {
	Entries []CatalogEntry `json:"entries"`
	Sources []SourceStatus `json:"sources"`
}

// Sources lists the built-in source (if configured) and the operator-added ones.
func (m *Marketplace) Sources(ctx context.Context) ([]Source, error) {
	stored, err := m.store.ListSources(ctx)
	if err != nil {
		return nil, err
	}
	out := []Source{}
	if m.builtin != nil {
		out = append(out, *m.builtin)
	}
	for _, s := range stored {
		if pub, err := ParsePublicKey(s.RootKey); err == nil {
			s.KeyPrint = Fingerprint(pub)
		}
		out = append(out, s)
	}
	return out, nil
}

func sourceID(rawURL, rootKey string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(rawURL) + "\n" + strings.TrimSpace(rootKey)))
	return "src-" + hex.EncodeToString(sum[:])[:12]
}

// AddSource configures an extra index source.
func (m *Marketplace) AddSource(ctx context.Context, name, rawURL, rootKey, actor string) (Source, error) {
	rawURL, rootKey = strings.TrimSpace(rawURL), strings.TrimSpace(rootKey)
	if err := httpsURL(rawURL); err != nil {
		return Source{}, invalid("the source URL %v", err)
	}
	if err := netguard.ValidateURL(ctx, rawURL); err != nil {
		return Source{}, invalid("the source URL is not allowed: %v", err)
	}
	pub, err := ParsePublicKey(rootKey)
	if err != nil {
		return Source{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		if u, perr := url.Parse(rawURL); perr == nil {
			name = u.Hostname()
		}
	}
	if name == "" || len(name) > 80 {
		return Source{}, invalid("a source name of 1 to 80 characters is required")
	}
	s := Source{ID: sourceID(rawURL, rootKey), Name: name, URL: rawURL, RootKey: rootKey, AddedAt: stamp(m.now()), AddedBy: actor, KeyPrint: Fingerprint(pub)}
	if m.builtin != nil && m.builtin.URL == rawURL && m.builtin.RootKey == rootKey {
		return Source{}, ErrSourceExists
	}
	if err := m.store.AddSource(ctx, s); err != nil {
		return Source{}, err
	}
	return s, nil
}

// RemoveSource drops an operator-added source and its cached index.
func (m *Marketplace) RemoveSource(ctx context.Context, id string) error {
	if id == BuiltinSourceID {
		return invalid("the default source is set by the environment and cannot be removed here")
	}
	return m.store.RemoveSource(ctx, id)
}

func errorKind(err error) string {
	switch {
	case errors.Is(err, ErrIndexSignature):
		return KindSignature
	case errors.Is(err, ErrIndexReplay):
		return KindReplay
	case errors.Is(err, ErrIndexTooLarge):
		return KindTooLarge
	case errors.Is(err, ErrIndexFormat):
		return KindFormat
	default:
		return KindFetch
	}
}

type loaded struct {
	idx    Index
	status SourceStatus
}

// load returns a source's verified index. A network failure serves the last
// verified copy marked stale; a verification failure serves nothing.
func (m *Marketplace) load(ctx context.Context, src Source, force bool) (loaded, error) {
	st, err := m.store.GetState(ctx, src.ID)
	if err != nil {
		return loaded{}, err
	}
	out := loaded{status: SourceStatus{Source: src, Status: "ok"}}
	root, err := ParsePublicKey(src.RootKey)
	if err != nil {
		return m.fail(ctx, out, st, ErrIndexSignature), nil
	}
	body, fresh, dirty := []byte(st.Body), false, false
	if force || st.Body == "" || m.now().Sub(parseStamp(st.FetchedAt)) > indexTTL {
		res, ferr := m.fetchIndex(ctx, src.URL, maxIndexBytes, st.ETag)
		switch {
		case ferr != nil && st.Body != "" && !errors.Is(ferr, ErrIndexTooLarge):
			out.status.Status = "stale"
			st.LastError, st.ErrorKind = ferr.Error(), KindFetch
			_ = m.store.SaveState(ctx, st)
		case ferr != nil:
			return m.fail(ctx, out, st, ferr), nil
		case res.NotModified:
			st.FetchedAt = stamp(m.now())
			st.LastError, st.ErrorKind = "", ""
			dirty = true
		default:
			body, fresh, dirty = res.Body, true, true
			st.ETag = res.ETag
		}
	}
	idx, hash, perr := ParseIndex(body, root)
	if perr != nil {
		return m.fail(ctx, out, st, perr), nil
	}
	if idx.Sequence < st.LastSequence || (idx.Sequence == st.LastSequence && st.LastHash != "" && hash != st.LastHash) {
		return m.fail(ctx, out, st, fmt.Errorf("%w (sequence %d, already saw %d)", ErrIndexReplay, idx.Sequence, st.LastSequence)), nil
	}
	if dirty {
		st.LastSequence, st.LastGenerated, st.LastHash, st.Body = idx.Sequence, idx.GeneratedAt, hash, string(body)
		if fresh {
			st.FetchedAt = stamp(m.now())
		}
		if err := m.store.SaveState(ctx, st); err != nil {
			return loaded{}, err
		}
	}
	out.idx = idx
	out.status.Sequence, out.status.GeneratedAt, out.status.FetchedAt, out.status.PluginCount = idx.Sequence, idx.GeneratedAt, st.FetchedAt, len(idx.Plugins)
	if out.status.Status == "stale" {
		out.status.Error, out.status.ErrorKind = st.LastError, st.ErrorKind
	}
	return out, nil
}

func (m *Marketplace) fail(ctx context.Context, out loaded, st SourceState, err error) loaded {
	st.LastError, st.ErrorKind = err.Error(), errorKind(err)
	_ = m.store.SaveState(ctx, st)
	out.status.Status, out.status.Error, out.status.ErrorKind = "error", st.LastError, st.ErrorKind
	out.status.Sequence, out.status.GeneratedAt, out.status.FetchedAt = st.LastSequence, st.LastGenerated, st.FetchedAt
	out.idx = Index{}
	return out
}

func parseStamp(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// Catalog merges every source. force refetches instead of using a cache under a day old.
func (m *Marketplace) Catalog(ctx context.Context, force bool) (Catalog, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sources, err := m.Sources(ctx)
	if err != nil {
		return Catalog{}, err
	}
	installed, keys := m.localState(ctx)
	cat := Catalog{Entries: []CatalogEntry{}, Sources: []SourceStatus{}}
	for _, src := range sources {
		l, err := m.load(ctx, src, force)
		if err != nil {
			return Catalog{}, err
		}
		cat.Sources = append(cat.Sources, l.status)
		for _, e := range l.idx.Plugins {
			cat.Entries = append(cat.Entries, m.catalogEntry(l.idx, src, e, installed, keys))
		}
	}
	return cat, nil
}

func (m *Marketplace) localState(ctx context.Context) (map[string]Record, []TrustedKey) {
	installed := map[string]Record{}
	var keys []TrustedKey
	if m.mgr == nil {
		return installed, keys
	}
	if recs, err := m.mgr.List(ctx); err == nil {
		for _, r := range recs {
			installed[r.Name] = r
		}
	}
	keys, _ = m.mgr.TrustKeys(ctx)
	return installed, keys
}

func (m *Marketplace) compatible(e Entry) bool {
	if e.MinVersion == "" || m.controlVersion == "" {
		return true
	}
	c, ok := semver.Compare(m.controlVersion, e.MinVersion)
	return !ok || c >= 0
}

func (m *Marketplace) catalogEntry(idx Index, src Source, e Entry, installed map[string]Record, keys []TrustedKey) CatalogEntry {
	ce := CatalogEntry{Entry: e, SourceID: src.ID, SourceName: src.Name, Verified: true, Compatible: m.compatible(e)}
	for _, k := range idx.PublisherKeys {
		if k.KeyID == e.PublisherKeyID {
			ce.PublisherKeyName = k.Name
		}
	}
	ce.KeyTrusted = slices.ContainsFunc(keys, func(k TrustedKey) bool { return k.KeyID == e.PublisherKeyID })
	if rec, ok := installed[e.ID]; ok {
		ce.Installed, ce.InstalledVersion = true, rec.Version
		ce.UpdateAvailable = rec.KeyID == e.PublisherKeyID && newerThan(e.Version, rec.Version) && ce.Compatible
	}
	return ce
}

func newerThan(candidate, installed string) bool {
	if c, ok := semver.Compare(candidate, installed); ok {
		return c > 0
	}
	return candidate != installed
}

// Refresh refetches one source, or all when sourceID is empty, and reports each status.
func (m *Marketplace) Refresh(ctx context.Context, sourceID string) ([]SourceStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sources, err := m.Sources(ctx)
	if err != nil {
		return nil, err
	}
	out := []SourceStatus{}
	for _, src := range sources {
		if sourceID != "" && src.ID != sourceID {
			continue
		}
		l, err := m.load(ctx, src, true)
		if err != nil {
			return nil, err
		}
		out = append(out, l.status)
	}
	if sourceID != "" && len(out) == 0 {
		return nil, ErrSourceUnknown
	}
	return out, nil
}

// resolved is a listing whose manifest has been fetched and checked against it.
type resolved struct {
	src      Source
	entry    Entry
	key      IndexKey
	manifest []byte
	man      Manifest
}

func (m *Marketplace) resolve(ctx context.Context, sourceID, id string) (resolved, error) {
	sources, err := m.Sources(ctx)
	if err != nil {
		return resolved{}, err
	}
	i := slices.IndexFunc(sources, func(s Source) bool { return s.ID == sourceID })
	if i < 0 {
		return resolved{}, ErrSourceUnknown
	}
	l, err := m.load(ctx, sources[i], false)
	if err != nil {
		return resolved{}, err
	}
	if l.status.Status == "error" {
		return resolved{}, fmt.Errorf("%s: %s", sources[i].Name, l.status.Error)
	}
	var entry *Entry
	for j := range l.idx.Plugins {
		if l.idx.Plugins[j].ID == id {
			entry = &l.idx.Plugins[j]
		}
	}
	if entry == nil {
		return resolved{}, ErrEntryNotFound
	}
	r := resolved{src: sources[i], entry: *entry}
	for _, k := range l.idx.PublisherKeys {
		if k.KeyID == entry.PublisherKeyID {
			r.key = k
		}
	}
	if !m.compatible(*entry) {
		return resolved{}, invalid("%s needs control plane %s or newer", entry.ID, entry.MinVersion)
	}
	raw, err := m.fetchFile(ctx, entry.ManifestURL, maxManifestBytes)
	if err != nil {
		return resolved{}, err
	}
	man, err := ParseManifest(raw)
	if err != nil {
		return resolved{}, err
	}
	if err := checkManifestMatchesEntry(man, *entry); err != nil {
		return resolved{}, err
	}
	pk := TrustedKey{KeyID: r.key.KeyID, Name: r.key.Name, PublicKey: r.key.PublicKey}
	if err := Verify([]TrustedKey{pk}, pk.KeyID, raw, man.Digest(), entry.ManifestSignature); err != nil {
		return resolved{}, fmt.Errorf("the manifest does not verify under the listed publisher key: %w", err)
	}
	r.manifest, r.man = raw, man
	return r, nil
}

func checkManifestMatchesEntry(man Manifest, e Entry) error {
	switch {
	case man.Name != e.ID:
		return invalid("the manifest is for %q, not the listed %q", man.Name, e.ID)
	case man.Version != e.Version:
		return invalid("the manifest is version %s, the listing says %s", man.Version, e.Version)
	case man.Digest() != e.Digest:
		return invalid("the manifest pins a different image digest than the listing")
	case !slices.Equal(man.Abilities, e.Abilities):
		return invalid("the manifest requests different abilities than the signed listing shows (%s)", strings.Join(man.Abilities, ", "))
	case !slices.Equal(man.OutboundHosts, normalizedHosts(e.OutboundHosts)):
		return invalid("the manifest declares different outbound hosts than the signed listing shows")
	}
	return nil
}

func normalizedHosts(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	return slices.Compact(out)
}

// MarketPreview is the confirm sheet: everything proven about a listing before any change.
type MarketPreview struct {
	Entry            Entry    `json:"entry"`
	SourceID         string   `json:"source_id"`
	SourceName       string   `json:"source_name"`
	KeyID            string   `json:"key_id"`
	KeyName          string   `json:"key_name"`
	KeyTrusted       bool     `json:"key_trusted"`
	Action           string   `json:"action"`
	InstalledVersion string   `json:"installed_version"`
	AddedAbilities   []string `json:"added_abilities"`
	AddedHosts       []string `json:"added_hosts"`
	Reapproval       bool     `json:"reapproval_required"`
}

func (m *Marketplace) trusted(ctx context.Context, keyID string) bool {
	if m.mgr == nil {
		return false
	}
	keys, _ := m.mgr.TrustKeys(ctx)
	return slices.ContainsFunc(keys, func(k TrustedKey) bool { return k.KeyID == keyID })
}

// existing returns the installed record this listing would replace, after the
// publisher and rollback checks.
func (m *Marketplace) existing(ctx context.Context, r resolved) (Record, bool, error) {
	if m.mgr == nil {
		return Record{}, false, nil
	}
	rec, err := m.mgr.Get(ctx, r.entry.ID)
	if errors.Is(err, ErrNotFound) {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, err
	}
	if rec.KeyID != r.entry.PublisherKeyID {
		return Record{}, false, invalid("%s is installed from a different publisher key, remove it first to install this listing", r.entry.ID)
	}
	if c, ok := semver.Compare(rec.Version, r.entry.Version); ok && c > 0 {
		return Record{}, false, invalid("%s %s is newer than the listed %s, refusing to roll back", rec.Name, rec.Version, r.entry.Version)
	}
	if rec.Digest == r.entry.Digest && rec.Version == r.entry.Version {
		return Record{}, false, invalid("%s %s is already installed", rec.Name, rec.Version)
	}
	return rec, true, nil
}

// Preview verifies a listing end to end and reports what installing it would do. It writes nothing.
func (m *Marketplace) Preview(ctx context.Context, sourceID, id string) (MarketPreview, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.resolve(ctx, sourceID, id)
	if err != nil {
		return MarketPreview{}, err
	}
	rec, upgrade, err := m.existing(ctx, r)
	if err != nil {
		return MarketPreview{}, err
	}
	p := MarketPreview{
		Entry: r.entry, SourceID: r.src.ID, SourceName: r.src.Name, KeyID: r.key.KeyID, KeyName: r.key.Name,
		KeyTrusted: m.trusted(ctx, r.key.KeyID), Action: "install", AddedAbilities: r.entry.Abilities, AddedHosts: r.man.OutboundHosts,
	}
	if upgrade {
		p.Action, p.InstalledVersion = "upgrade", rec.Version
		p.AddedAbilities, _ = diff(rec.RequestedAbilities, r.man.Abilities)
		eff, _ := EffectiveOutboundHosts(r.man, rec.ConfigJSON)
		p.AddedHosts, _ = diff(rec.OutboundHosts, eff)
		p.Reapproval = len(p.AddedAbilities) > 0 || len(p.AddedHosts) > 0
	}
	return p, nil
}

// MarketInstallRequest is the operator's confirmation of exactly what the sheet showed.
type MarketInstallRequest struct {
	SourceID  string   `json:"source_id"`
	ID        string   `json:"id"`
	Version   string   `json:"version"`
	Digest    string   `json:"digest"`
	Abilities []string `json:"abilities"`
	// TrustKeyID must equal the publisher key fingerprint when that key is not yet trusted.
	TrustKeyID string `json:"trust_key_id"`
}

// MarketInstallResult reports what the one-click flow did.
type MarketInstallResult struct {
	Plugin     Record `json:"plugin"`
	Action     string `json:"action"`
	TrustAdded bool   `json:"trust_added"`
}

// Install trusts the publisher key if confirmed, installs or upgrades, approves
// exactly the confirmed abilities and enables, rolling back what it added on failure.
func (m *Marketplace) Install(ctx context.Context, req MarketInstallRequest, actor string) (MarketInstallResult, error) {
	if m.mgr == nil {
		return MarketInstallResult{}, ErrNoRuntime
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.resolve(ctx, req.SourceID, req.ID)
	if err != nil {
		return MarketInstallResult{}, err
	}
	shown, err := normalizeAbilities(req.Abilities)
	if err != nil {
		return MarketInstallResult{}, err
	}
	if req.Version != r.entry.Version || req.Digest != r.entry.Digest || !slices.Equal(shown, r.entry.Abilities) {
		return MarketInstallResult{}, ErrChanged
	}
	_, upgrade, err := m.existing(ctx, r)
	if err != nil {
		return MarketInstallResult{}, err
	}
	res := MarketInstallResult{Action: "install"}
	if !m.trusted(ctx, r.key.KeyID) {
		if req.TrustKeyID != r.key.KeyID {
			return MarketInstallResult{}, invalid("trust the publisher key %s to install from this source", r.key.KeyID)
		}
		name := r.key.Name
		if limit := 80 - len(marketKeySuffix); len(name) > limit {
			name = name[:limit]
		}
		if _, err := m.mgr.TrustAdd(ctx, name+marketKeySuffix, r.key.PublicKey, actor); err != nil {
			_ = m.mgr.TrustRemove(ctx, r.key.KeyID, actor)
			return MarketInstallResult{}, err
		}
		res.TrustAdded = true
	}
	ir := InstallRequest{Manifest: r.manifest, Signature: r.entry.ManifestSignature, KeyID: r.key.KeyID, ExpectedDigest: r.entry.Digest, Approve: shown}
	if upgrade {
		res.Action = "upgrade"
		res.Plugin, err = m.mgr.Upgrade(ctx, ir, actor)
	} else {
		res.Plugin, err = m.mgr.Install(ctx, ir, actor)
		if err == nil {
			res.Plugin, err = m.mgr.Enable(ctx, r.entry.ID, actor)
			if err != nil {
				_ = m.mgr.Remove(ctx, r.entry.ID, actor)
			}
		}
	}
	if err != nil {
		if res.TrustAdded {
			_ = m.mgr.TrustRemove(ctx, r.key.KeyID, actor)
		}
		return MarketInstallResult{}, err
	}
	aerr := m.mgr.audit.Record(ctx, AuditEvent{Action: AuditMarketplaceInstall, Actor: actor, Plugin: r.entry.ID, Detail: map[string]string{
		"source": r.src.ID, "version": r.entry.Version, "digest": r.entry.Digest, "key_id": r.key.KeyID,
		"approved": strings.Join(shown, ","), "trust_added": boolString(res.TrustAdded), "action": res.Action,
	}})
	return res, aerr
}
