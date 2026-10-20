package api

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/selfupgrade"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

const (
	schemaFetchTimeout = 12 * time.Second
	schemaFailureTTL   = 2 * time.Minute
	cosignOIDCIssuer   = "https://token.actions.githubusercontent.com"
)

// ReleaseSchemaFetcher returns the verified schema version a release tag supports.
type ReleaseSchemaFetcher func(ctx context.Context, tag string) (selfupgrade.SchemaResult, error)

var errTrustRootNotReady = errors.New("sigstore trust data is not ready yet")

// NewReleaseSchemaFetcher builds the production fetcher: release manifest,
// signed checksums and bundle from the project's GitHub releases only.
func NewReleaseSchemaFetcher(dataDir string, trust TrustRootProbe) ReleaseSchemaFetcher {
	fetcher := selfupgrade.Fetcher{
		BaseURL:       "https://github.com/" + githubRepo + "/releases/download",
		RedirectHosts: []string{"github.com", "githubusercontent.com"},
	}
	return func(ctx context.Context, tag string) (selfupgrade.SchemaResult, error) {
		_, pinned, _ := selfupgrade.ReadPin(dataDir)
		if trust != nil {
			if state := trust.State(); state != upgrade.TrustRootReady && state != upgrade.TrustRootCached {
				trust.Warm()
				return selfupgrade.SchemaResult{}, errTrustRootNotReady
			}
		}
		signer := selfupgrade.Signer{
			Mode:           selfupgrade.VerifyAuto,
			IdentityRegexp: "(?i)https://github.com/" + regexp.QuoteMeta(githubRepo) + `/\.github/workflows/release\.yml@.*`,
			Issuer:         cosignOIDCIssuer,
			Pinned:         pinned,
			TUFCacheDir:    selfupgrade.TUFCacheDir(dataDir),
		}
		return selfupgrade.FetchReleaseSchema(ctx, fetcher, signer, tag, "")
	}
}

type schemaFailure struct {
	at    time.Time
	state string
	text  string
}

// releaseSchemaSource caches verified release schemas (immutable per tag) and
// briefly remembers failures so a down GitHub is not hit on every poll.
type releaseSchemaSource struct {
	fetch  ReleaseSchemaFetcher
	mu     sync.Mutex
	known  map[string]int
	failed map[string]schemaFailure
}

// WithReleaseSchemaSource turns on the pre-click schema compatibility check.
func WithReleaseSchemaSource(f ReleaseSchemaFetcher) Option {
	return func(rt *Router) {
		rt.schemaSource = &releaseSchemaSource{fetch: f, known: map[string]int{}, failed: map[string]schemaFailure{}}
	}
}

func (s *releaseSchemaSource) forget() {
	s.mu.Lock()
	s.failed = map[string]schemaFailure{}
	s.mu.Unlock()
}

// resolve reports the release's schema state. assets is the release's asset
// list when known: no manifest in it means the release predates the manifest.
func (s *releaseSchemaSource) resolve(ctx context.Context, tag string, assets []string, dbSchema int) upgrade.SchemaInput {
	in := upgrade.SchemaInput{Tag: tag, DB: dbSchema, Release: -1, State: upgrade.SchemaMissing}
	if assets != nil && !hasAsset(assets, upgrade.ManifestAsset) {
		return in
	}
	s.mu.Lock()
	if v, ok := s.known[tag]; ok {
		s.mu.Unlock()
		in.State, in.Release = upgrade.SchemaKnown, v
		return in
	}
	if f, ok := s.failed[tag]; ok && time.Since(f.at) < schemaFailureTTL {
		s.mu.Unlock()
		in.State, in.Detail = f.state, f.text
		return in
	}
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, schemaFetchTimeout)
	defer cancel()
	res, err := s.fetch(ctx, tag)
	if err == nil {
		s.mu.Lock()
		s.known[tag] = res.Schema
		s.mu.Unlock()
		in.State, in.Release = upgrade.SchemaKnown, res.Schema
		return in
	}
	state, text := classifySchemaError(err)
	s.mu.Lock()
	s.failed[tag] = schemaFailure{at: time.Now(), state: state, text: text}
	s.mu.Unlock()
	in.State, in.Detail = state, text
	return in
}

func classifySchemaError(err error) (state, text string) {
	switch {
	case errors.Is(err, errTrustRootNotReady):
		return upgrade.SchemaUnavailable, ""
	case errors.Is(err, selfupgrade.ErrChecksumMismatch), errors.Is(err, selfupgrade.ErrChecksumMissing),
		errors.Is(err, selfupgrade.ErrSignatureInvalid), errors.Is(err, selfupgrade.ErrSignaturePinned),
		errors.Is(err, selfupgrade.ErrSignatureRequired), errors.Is(err, selfupgrade.ErrManifestInvalid):
		return upgrade.SchemaUnverified, err.Error()
	default:
		return upgrade.SchemaUnavailable, ""
	}
}

func githubAssetNames(r *githubRelease) []string {
	names := make([]string, 0, len(r.Assets))
	for _, a := range r.Assets {
		names = append(names, a.Name)
	}
	return names
}

func hasAsset(assets []string, name string) bool {
	for _, a := range assets {
		if a == name {
			return true
		}
	}
	return false
}

// schemaInputFor resolves the target's schema compatibility, or nil when the
// check is not configured or does not apply.
func (rt *Router) schemaInputFor(ctx context.Context, tag string, assets []string) *upgrade.SchemaInput {
	if rt.schemaSource == nil || !selfupgrade.ValidTag(tag) {
		return nil
	}
	in := rt.schemaSource.resolve(ctx, tag, assets, rt.liveSchemaVersion(ctx))
	if in.State == upgrade.SchemaUnverified {
		rt.logger.Warn("api: release schema manifest not verified", slog.String("tag", tag), slog.String("reason", in.Detail))
	}
	return &in
}

// schemaRefusesRelease reports whether the target is known to be older than
// the database, so the release list must not offer it.
func (rt *Router) schemaRefusesRelease(ctx context.Context, tag string, assets []string) bool {
	in := rt.schemaInputFor(ctx, tag, assets)
	return in != nil && in.State == upgrade.SchemaKnown && in.DB >= 0 && in.Release < in.DB
}
