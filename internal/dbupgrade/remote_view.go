package dbupgrade

import (
	"errors"
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbcatalogbuild"
)

type remoteEngine struct {
	versions []string
	lines    []SupportLine
}

// ApplyRemote replaces what remote data adds to one engine. Only versions and
// end-of-life dates are taken: auto_max, image_revert, levels and advisories
// stay curated. An empty Engine clears the remote data.
func (c *Catalog) ApplyRemote(engine string, e dbcatalogbuild.Engine) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.remote == nil {
		c.remote = map[string]remoteEngine{}
	}
	ec, ok := c.engines[engine]
	if !ok || (len(e.Versions) == 0 && len(e.Lines) == 0) {
		delete(c.remote, engine)
		return
	}
	var tags []string
	for _, v := range e.Versions {
		if (v.Channel == dbcatalogbuild.ChannelStable || v.Channel == dbcatalogbuild.ChannelLTS) && acceptsVersion(engine, v.Version) {
			tags = append(tags, v.Version)
		}
	}
	re := remoteEngine{versions: usableTags(ec, tags)}
	for _, l := range e.Lines {
		if _, ok := parseVersion(l.Line); ok {
			re.lines = append(re.lines, SupportLine{Line: l.Line, EOL: l.EOL})
		}
	}
	c.remote[engine] = re
}

func (c *Catalog) withRemote(id string, e EngineCatalog) EngineCatalog {
	r, ok := c.remote[id]
	if !ok {
		return e
	}
	lines := append([]SupportLine(nil), e.Lines...)
	for _, rl := range r.lines {
		found := false
		for i := range lines {
			if lines[i].Line == rl.Line {
				lines[i].EOL, found = rl.EOL, true
			}
		}
		if !found {
			lines = append(lines, rl)
		}
	}
	e.Lines = lines
	if len(r.versions) > 0 {
		seen := map[string]bool{}
		merged := make([]string, 0, len(e.Versions)+len(r.versions))
		for _, v := range append(append([]string{}, e.Versions...), r.versions...) {
			if !seen[v] {
				seen[v] = true
				merged = append(merged, v)
			}
		}
		sort.Strings(merged)
		e.Versions = merged
	}
	return e
}

// VersionInfo is one selectable version with its channel.
type VersionInfo struct {
	Version     string `json:"version"`
	Channel     string `json:"channel"`
	Released    string `json:"released,omitempty"`
	EOL         string `json:"eol,omitempty"`
	Recommended bool   `json:"recommended"`
}

// VersionsView is the version list of one engine with its provenance.
type VersionsView struct {
	Engine      string                `json:"engine"`
	Origin      string                `json:"origin"`
	Versions    []VersionInfo         `json:"versions"`
	Source      dbcatalogbuild.Source `json:"source"`
	GeneratedAt string                `json:"generated_at,omitempty"`
	FetchedAt   string                `json:"fetched_at,omitempty"`
	Freshness   string                `json:"freshness"`
	Message     string                `json:"message,omitempty"`
}

// ErrUnknownEngine means the catalog has no such engine.
var ErrUnknownEngine = errors.New("dbupgrade: unknown engine")

// Versions returns an engine's version list with channels and sources.
func (r *RemoteCatalog) Versions(engine string) (VersionsView, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.versionsLocked(engine)
}

func (r *RemoteCatalog) versionsLocked(engine string) (VersionsView, error) {
	if r.cfg.Catalog == nil {
		return VersionsView{}, ErrUnknownEngine
	}
	ec, ok := r.cfg.Catalog.Engine(engine)
	if !ok {
		return VersionsView{}, ErrUnknownEngine
	}
	now := r.cfg.Now()
	data, origin, gen, fetched, have := r.engineData(engine)
	meta := map[string]dbcatalogbuild.Version{}
	for _, v := range data.Versions {
		meta[v.Version] = v
	}
	today := now.Format("2006-01-02")
	lineEOL := func(version string) string {
		pv, ok := parseVersion(version)
		if !ok {
			return ""
		}
		key := lineKey(ec.Levels, pv)
		for _, l := range ec.Lines {
			if l.Line == key {
				return l.EOL
			}
		}
		return ""
	}
	seen := map[string]bool{}
	var out []VersionInfo
	for _, v := range ec.Versions {
		seen[v] = true
		info := VersionInfo{Version: v, Channel: dbcatalogbuild.ChannelStable, EOL: lineEOL(v)}
		if m, ok := meta[v]; ok {
			info.Channel, info.Released = m.Channel, m.Released
			if m.EOL != "" {
				info.EOL = m.EOL
			}
		}
		if info.EOL != "" && info.EOL <= today {
			info.Channel = dbcatalogbuild.ChannelEOL
		}
		out = append(out, info)
	}
	for _, m := range data.Versions {
		if seen[m.Version] || !acceptsVersion(engine, m.Version) {
			continue
		}
		out = append(out, VersionInfo{Version: m.Version, Channel: m.Channel, Released: m.Released, EOL: m.EOL})
	}
	markRecommended(out, meta)
	sort.SliceStable(out, func(i, j int) bool { return versionDesc(out[i].Version, out[j].Version) })

	view := VersionsView{Engine: engine, Origin: origin, Versions: out, Freshness: FreshnessStale}
	if have {
		view.Source = data.Source
	}
	if view.Source == (dbcatalogbuild.Source{}) {
		if spec, ok := dbcatalogbuild.SpecFor(engine); ok {
			view.Source = spec.Source()
		}
	}
	if origin == CatalogSourceEmbedded {
		gen = parseDate(r.cfg.Catalog.Updated())
		view.GeneratedAt = r.cfg.Catalog.Updated()
	} else {
		view.GeneratedAt = gen.UTC().Format(time.RFC3339)
		view.FetchedAt = fetched.UTC().Format(time.RFC3339)
	}
	view.Freshness = FreshnessOf(gen, now)
	return view, nil
}

func parseDate(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func markRecommended(out []VersionInfo, meta map[string]dbcatalogbuild.Version) {
	for i := range out {
		if m, ok := meta[out[i].Version]; ok && m.Recommended && isSelectable(out[i].Channel) {
			out[i].Recommended = true
			return
		}
	}
	best := -1
	for i := range out {
		if !isSelectable(out[i].Channel) {
			continue
		}
		if best < 0 || versionDesc(out[i].Version, out[best].Version) {
			best = i
		}
	}
	if best >= 0 {
		out[best].Recommended = true
	}
}

func isSelectable(channel string) bool {
	return channel == dbcatalogbuild.ChannelStable || channel == dbcatalogbuild.ChannelLTS
}

// versionDesc reports whether a is newer than b, falling back to string order.
func versionDesc(a, b string) bool {
	pa, okA := parseVersion(a)
	pb, okB := parseVersion(b)
	if okA && okB {
		if c := compareVersions(pa, pb); c != 0 {
			return c > 0
		}
	}
	return a > b
}

// Status describes where the catalog data currently comes from.
type Status struct {
	Source              string            `json:"source"`
	Mode                string            `json:"mode"`
	RemoteEnabled       bool              `json:"remote_enabled"`
	DisabledReason      string            `json:"disabled_reason,omitempty"`
	URL                 string            `json:"url"`
	GeneratedAt         string            `json:"generated_at,omitempty"`
	FetchedAt           string            `json:"fetched_at,omitempty"`
	LastChecked         string            `json:"last_checked,omitempty"`
	NextAttempt         string            `json:"next_attempt,omitempty"`
	LastError           string            `json:"last_error,omitempty"`
	Freshness           string            `json:"freshness"`
	ServingStaleCopy    bool              `json:"serving_stale_copy"`
	EmbeddedUpdated     string            `json:"embedded_updated"`
	LiveChecks          map[string]string `json:"live_checks,omitempty"`
	PublicKeyConfigured bool              `json:"public_key_configured"`
}

// Status reports the active source, ages and the last error.
func (r *RemoteCatalog) Status() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.cfg.Now()
	f, origin, fetched := r.base()
	s := Status{
		Source: origin, Mode: r.cfg.Source, URL: r.cfg.URL, LastError: r.st.LastError,
		PublicKeyConfigured: r.cfg.PublicKey != "", Freshness: FreshnessStale,
		EmbeddedUpdated: r.cfg.Catalog.Updated(),
	}
	if s.Mode == CatalogSourceDirect {
		s.URL = "registries and endoflife.date (direct)"
	}
	s.DisabledReason = r.publishedOffReason()
	if r.cfg.Source == CatalogSourceDirect {
		s.DisabledReason = r.remoteOffReason()
	}
	s.RemoteEnabled = s.DisabledReason == ""
	ref := parseDate(r.cfg.Catalog.Updated())
	if f != nil {
		gen, _ := time.Parse(time.RFC3339, f.GeneratedAt)
		if gen.IsZero() {
			gen = fetched
		}
		ref = gen
		s.GeneratedAt = gen.UTC().Format(time.RFC3339)
		s.FetchedAt = fetched.UTC().Format(time.RFC3339)
	} else {
		s.GeneratedAt = r.cfg.Catalog.Updated()
		s.ServingStaleCopy = false
		if (r.st.PublishedRaw != "" || r.st.Direct != nil) && s.LastError == "" {
			s.LastError = "the cached catalog is older than 90 days, using the embedded catalog"
		}
	}
	if !r.st.LastChecked.IsZero() {
		s.LastChecked = r.st.LastChecked.UTC().Format(time.RFC3339)
		s.ServingStaleCopy = f != nil && r.st.LastError != "" && now.Sub(r.st.LastChecked) > publishedInterval
	}
	if !r.st.NextAttempt.IsZero() {
		s.NextAttempt = r.st.NextAttempt.UTC().Format(time.RFC3339)
	}
	for id, le := range r.st.Live {
		if s.LiveChecks == nil {
			s.LiveChecks = map[string]string{}
		}
		s.LiveChecks[id] = le.FetchedAt.UTC().Format(time.RFC3339)
		if le.FetchedAt.After(fetched) {
			s.Source = CatalogSourceLive
		}
	}
	s.Freshness = FreshnessOf(ref, now)
	return s
}
