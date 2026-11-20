package build

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/docker/cli/cli/config/types"
	bkclient "github.com/moby/buildkit/client"
	"github.com/moby/buildkit/session/auth/authprovider"
)

// SettingTransferMode is the runtime setting key that picks how a remote
// build's image reaches the control plane; the env var only seeds it.
const (
	SettingTransferMode = "build.transfer_mode"
	EnvTransferMode     = "APP_BUILD_TRANSFER_MODE"
)

// TransferMode is how a dispatched build returns its image.
type TransferMode string

// TransferTar streams a docker-save tar back (today's path); TransferRegistry pushes by digest.
const (
	TransferTar      TransferMode = "tar"
	TransferRegistry TransferMode = "registry"
)

// ParseTransferMode maps an empty value to tar so nothing changes until enabled.
func ParseTransferMode(s string) (TransferMode, error) {
	switch TransferMode(strings.ToLower(strings.TrimSpace(s))) {
	case "", TransferTar:
		return TransferTar, nil
	case TransferRegistry:
		return TransferRegistry, nil
	default:
		return "", fmt.Errorf("build: %s must be %q or %q, got %q", SettingTransferMode, TransferTar, TransferRegistry, s)
	}
}

// Token errors are distinct so a rejected push says why without echoing the token.
var (
	ErrPushTokenInvalid = errors.New("build: push token is not valid")
	ErrPushTokenExpired = errors.New("build: push token has expired")
	ErrPushTokenScope   = errors.New("build: push token does not grant this repository, tag or action")
)

const pushActionPush = "push"

// pushUser is the fixed basic-auth user; the token is the password.
const pushUser = "levelrail-job"

// pushTokenTyp keeps these MACs from being confused with any other token on the same key.
const pushTokenTyp = "levelrail-push"

// ErrPushTokenRevoked means the job finished and the token no longer works.
var ErrPushTokenRevoked = errors.New("build: push token was revoked")

type pushClaims struct {
	Typ   string `json:"typ"`
	Aud   string `json:"aud"`
	Repo  string `json:"r"`
	Tag   string `json:"t"`
	Act   string `json:"a"`
	Exp   int64  `json:"e"`
	Nonce string `json:"n"`
	// Cache is an optional second repository (the build cache) the same token may push and pull.
	Cache string `json:"c,omitempty"`
}

// PushTokenIssuer mints and checks job-scoped push tokens. They are
// stateless HMAC tokens, so nothing is stored and a restart key rotation revokes all.
type PushTokenIssuer struct {
	key []byte
	now func() time.Time
	mu  sync.Mutex
	// revoked maps a nonce to its expiry so the set stays small.
	revoked map[string]int64
}

// NewPushTokenIssuer requires a key of at least 32 bytes.
func NewPushTokenIssuer(key []byte) (*PushTokenIssuer, error) {
	if len(key) < 32 {
		return nil, errors.New("build: push token key must be at least 32 bytes")
	}
	return &PushTokenIssuer{key: append([]byte(nil), key...), now: time.Now, revoked: map[string]int64{}}, nil
}

func (i *PushTokenIssuer) mac(payload string) []byte {
	h := hmac.New(sha256.New, i.key)
	h.Write([]byte(payload))
	return h.Sum(nil)
}

// Issue grants push on exactly one repo and tag until now+ttl.
func (i *PushTokenIssuer) Issue(repo, tag string, ttl time.Duration) (string, error) {
	return i.IssueWithCache(repo, tag, "", ttl)
}

// IssueWithCache is Issue plus access to one cache repository, so a builder needs no second credential.
func (i *PushTokenIssuer) IssueWithCache(repo, tag, cacheRepo string, ttl time.Duration) (string, error) {
	if repo == "" || tag == "" || ttl <= 0 {
		return "", errors.New("build: push token needs a repo, a tag and a positive ttl")
	}
	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("build: push token nonce: %w", err)
	}
	raw, err := json.Marshal(pushClaims{Typ: pushTokenTyp, Aud: pushTokenTyp, Repo: repo, Tag: tag, Act: pushActionPush, Exp: i.now().Add(ttl).Unix(), Nonce: hex.EncodeToString(nonce), Cache: cacheRepo})
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	return payload + "." + base64.RawURLEncoding.EncodeToString(i.mac(payload)), nil
}

// parse checks signature, type, expiry and revocation, in that order.
func (i *PushTokenIssuer) parse(token string) (pushClaims, error) {
	var c pushClaims
	payload, sig, ok := strings.Cut(token, ".")
	if !ok {
		return c, ErrPushTokenInvalid
	}
	want, err := base64.RawURLEncoding.Strict().DecodeString(sig)
	if err != nil || subtle.ConstantTimeCompare(want, i.mac(payload)) != 1 {
		return c, ErrPushTokenInvalid
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(payload)
	if err != nil {
		return c, ErrPushTokenInvalid
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil || c.Typ != pushTokenTyp || c.Aud != pushTokenTyp {
		return pushClaims{}, ErrPushTokenInvalid
	}
	if i.now().Unix() >= c.Exp {
		return pushClaims{}, ErrPushTokenExpired
	}
	i.mu.Lock()
	_, gone := i.revoked[c.Nonce]
	i.mu.Unlock()
	if gone {
		return pushClaims{}, ErrPushTokenRevoked
	}
	return c, nil
}

// Verify accepts the token only for push on exactly repo and tag, before expiry.
func (i *PushTokenIssuer) Verify(token, repo, tag, action string) error {
	c, err := i.parse(token)
	if err != nil {
		return err
	}
	if action != pushActionPush || c.Act != pushActionPush || c.Repo != repo || c.Tag != tag {
		return ErrPushTokenScope
	}
	return nil
}

// VerifyRepo is Verify for registry scopes, which name a repository but never a tag.
func (i *PushTokenIssuer) VerifyRepo(token, repo string) (time.Time, error) {
	exp, _, err := i.verifyScope(token, repo)
	return exp, err
}

// verifyScope also reports whether repo matched the cache repository rather than the job repository.
func (i *PushTokenIssuer) verifyScope(token, repo string) (time.Time, bool, error) {
	c, err := i.parse(token)
	if err != nil {
		return time.Time{}, false, err
	}
	if c.Act != pushActionPush {
		return time.Time{}, false, ErrPushTokenScope
	}
	switch {
	case c.Repo == repo:
		return time.Unix(c.Exp, 0), false, nil
	case c.Cache != "" && c.Cache == repo:
		return time.Unix(c.Exp, 0), true, nil
	}
	return time.Time{}, false, ErrPushTokenScope
}

// Nonce is the token's per-job id for rate limiting, or "" when the token does not verify.
func (i *PushTokenIssuer) Nonce(token string) string {
	c, err := i.parse(token)
	if err != nil {
		return ""
	}
	return c.Nonce
}

// Revoke ends a token's life when its job finishes; unknown tokens are ignored.
func (i *PushTokenIssuer) Revoke(token string) {
	c, err := i.parse(token)
	if err != nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	now := i.now().Unix()
	for n, exp := range i.revoked {
		if exp <= now {
			delete(i.revoked, n)
		}
	}
	i.revoked[c.Nonce] = c.Exp
}

// PushGrant tells a builder where to push and with what credential.
type PushGrant struct {
	// Ref is host/repo:tag.
	Ref      string
	Token    string
	Insecure bool
}

// LogValue keeps the token out of every structured log line.
func (g PushGrant) LogValue() slog.Value {
	return slog.GroupValue(slog.String("ref", g.Ref), slog.String("token", "[redacted]"))
}

// String redacts the token for fmt verbs.
func (g PushGrant) String() string { return "PushGrant{" + g.Ref + ", token redacted}" }

// GoString redacts the token for %#v.
func (g PushGrant) GoString() string { return g.String() }

// SplitRef returns repo (host/path) and tag of a "host/path:tag" ref.
func SplitRef(ref string) (repo, tag string, err error) {
	slash := strings.LastIndex(ref, "/")
	colon := strings.LastIndex(ref, ":")
	if slash < 0 || colon <= slash || colon == len(ref)-1 {
		return "", "", fmt.Errorf("build: registry ref %q must be host/repo:tag", ref)
	}
	return ref[:colon], ref[colon+1:], nil
}

var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// DigestVerifier asks the control plane's registry which digest a tag points at.
type DigestVerifier interface {
	// ManifestDigest returns the registry's Docker-Content-Digest, or "" when the manifest is absent.
	ManifestDigest(ctx context.Context, repo, reference string) (string, error)
}

// ErrDigestMissing means the builder reported a digest the registry does not hold.
var ErrDigestMissing = errors.New("build: reported image digest is not in the registry")

// VerifyPushedDigest fails closed: the registry's answer for the tag must equal
// the builder's digest, so a builder cannot name a digest it did not push.
func VerifyPushedDigest(ctx context.Context, v DigestVerifier, ref, digest string) error {
	if !digestPattern.MatchString(digest) {
		return fmt.Errorf("build: builder reported a malformed digest %q", digest)
	}
	repo, tag, err := SplitRef(ref)
	if err != nil {
		return err
	}
	got, err := v.ManifestDigest(ctx, repo, tag)
	if err != nil {
		return fmt.Errorf("build: verify pushed digest: %w", err)
	}
	if got == "" {
		return ErrDigestMissing
	}
	if got != digest {
		return fmt.Errorf("build: registry digest does not match the builder's report: %w", ErrDigestMissing)
	}
	return nil
}

// applyPush turns a solve's docker-tar export into a registry push by digest
// and attaches the job token as the only registry credential.
func applyPush(opt *bkclient.SolveOpt, g *PushGrant, cache CacheConfig) error {
	if g == nil {
		return nil
	}
	repo, _, err := SplitRef(g.Ref)
	if err != nil {
		return err
	}
	if len(opt.Exports) != 1 {
		return errors.New("build: push expects exactly one export")
	}
	attrs := opt.Exports[0].Attrs
	attrs["name"] = g.Ref
	attrs["push"] = "true"
	if g.Insecure {
		attrs["registry.insecure"] = "true"
	}
	opt.Exports[0] = bkclient.ExportEntry{Type: bkclient.ExporterImage, Attrs: attrs}
	host := repo[:strings.Index(repo, "/")]
	provider := pushAuthConfigWithCache(host, repo[len(host)+1:], cacheRepoPath(cache, host), g.Token)
	opt.Session = append(opt.Session, authprovider.NewDockerAuthProvider(authprovider.DockerAuthProviderConfig{
		AuthConfigProvider: func(_ context.Context, h string, scopes []string, _ authprovider.ExpireCachedAuthCheck) (types.AuthConfig, error) {
			return provider(h, scopes), nil
		},
	}))
	return nil
}

// pushAuthConfig releases the job token only for push+pull on exactly the job repo, compared as a
// set of actions since clients differ in order. Nil scopes (the basic-credential lookup for the
// token realm) get it for the registry host only; the realm still checks scope itself.
func pushAuthConfig(host, jobRepo, token string) func(h string, scopes []string) types.AuthConfig {
	return pushAuthConfigWithCache(host, jobRepo, "", token)
}

// CacheAuth lets a build on the control plane use the registry cache while the registry runs token auth.
type CacheAuth struct {
	Token string
}

// String keeps the token out of logs.
func (a CacheAuth) String() string { return "CacheAuth{token redacted}" }

// GoString keeps the token out of %#v output.
func (a CacheAuth) GoString() string { return a.String() }

// cacheRepoPath is the cache repository path on host, or "" when the cache lives elsewhere.
func cacheRepoPath(cache CacheConfig, host string) string {
	if cache.RegistryRef == "" {
		return ""
	}
	repo, _, err := SplitRef(cache.RegistryRef)
	if err != nil || !strings.HasPrefix(repo, host+"/") {
		return ""
	}
	return repo[len(host)+1:]
}

// applyCacheAuth attaches the cache-only token for builds that export a tar and push nothing.
func applyCacheAuth(opt *bkclient.SolveOpt, a *CacheAuth, cache CacheConfig) {
	if a == nil || a.Token == "" || cache.RegistryRef == "" {
		return
	}
	repo, _, err := SplitRef(cache.RegistryRef)
	if err != nil {
		return
	}
	host := repo[:strings.Index(repo, "/")]
	provider := pushAuthConfigWithCache(host, "", repo[len(host)+1:], a.Token)
	opt.Session = append(opt.Session, authprovider.NewDockerAuthProvider(authprovider.DockerAuthProviderConfig{
		AuthConfigProvider: func(_ context.Context, h string, scopes []string, _ authprovider.ExpireCachedAuthCheck) (types.AuthConfig, error) {
			return provider(h, scopes), nil
		},
	}))
}

// pushAuthConfigWithCache also releases the token for the cache repository, pull alone or push and pull.
func pushAuthConfigWithCache(host, jobRepo, cacheRepo, token string) func(h string, scopes []string) types.AuthConfig {
	grant := types.AuthConfig{Username: pushUser, Password: token}
	return func(h string, scopes []string) types.AuthConfig {
		if h != host {
			return types.AuthConfig{}
		}
		if len(scopes) == 0 {
			return grant
		}
		if len(scopes) == 1 && jobRepo != "" && scopeIsJobPushPull(scopes[0], jobRepo) {
			return grant
		}
		if len(scopes) == 1 && cacheRepo != "" && scopeIsCacheAccess(scopes[0], cacheRepo) {
			return grant
		}
		return types.AuthConfig{}
	}
}

func scopeIsJobPushPull(scope, jobRepo string) bool {
	scope = strings.TrimPrefix(scope, "repository:")
	name, actions, ok := strings.Cut(scope, ":")
	if !ok || name != jobRepo {
		return false
	}
	set := map[string]bool{}
	for _, a := range strings.Split(actions, ",") {
		set[a] = true
	}
	return len(set) == 2 && set[pushActionPush] && set["pull"]
}

// scopeIsCacheAccess accepts pull, or push with pull, on exactly the cache repository.
func scopeIsCacheAccess(scope, cacheRepo string) bool {
	scope = strings.TrimPrefix(scope, "repository:")
	name, actions, ok := strings.Cut(scope, ":")
	if !ok || name != cacheRepo {
		return false
	}
	for _, a := range strings.Split(actions, ",") {
		if a != "pull" && a != pushActionPush {
			return false
		}
	}
	return actions != ""
}
