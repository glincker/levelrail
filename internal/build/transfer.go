package build

import (
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

type pushClaims struct {
	Repo  string `json:"r"`
	Tag   string `json:"t"`
	Act   string `json:"a"`
	Exp   int64  `json:"e"`
	Nonce string `json:"n"`
}

// PushTokenIssuer mints and checks job-scoped push tokens. They are
// stateless HMAC tokens, so nothing is stored and a restart key rotation revokes all.
type PushTokenIssuer struct {
	key []byte
	now func() time.Time
}

// NewPushTokenIssuer requires a key of at least 32 bytes.
func NewPushTokenIssuer(key []byte) (*PushTokenIssuer, error) {
	if len(key) < 32 {
		return nil, errors.New("build: push token key must be at least 32 bytes")
	}
	return &PushTokenIssuer{key: append([]byte(nil), key...), now: time.Now}, nil
}

func (i *PushTokenIssuer) mac(payload string) []byte {
	h := hmac.New(sha256.New, i.key)
	h.Write([]byte(payload))
	return h.Sum(nil)
}

// Issue grants push on exactly one repo and tag until now+ttl.
func (i *PushTokenIssuer) Issue(repo, tag string, ttl time.Duration) (string, error) {
	if repo == "" || tag == "" || ttl <= 0 {
		return "", errors.New("build: push token needs a repo, a tag and a positive ttl")
	}
	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("build: push token nonce: %w", err)
	}
	raw, err := json.Marshal(pushClaims{Repo: repo, Tag: tag, Act: pushActionPush, Exp: i.now().Add(ttl).Unix(), Nonce: hex.EncodeToString(nonce)})
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	return payload + "." + base64.RawURLEncoding.EncodeToString(i.mac(payload)), nil
}

// Verify accepts the token only for push on exactly repo and tag, before expiry.
func (i *PushTokenIssuer) Verify(token, repo, tag, action string) error {
	payload, sig, ok := strings.Cut(token, ".")
	if !ok {
		return ErrPushTokenInvalid
	}
	want, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || subtle.ConstantTimeCompare(want, i.mac(payload)) != 1 {
		return ErrPushTokenInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return ErrPushTokenInvalid
	}
	var c pushClaims
	if err := json.Unmarshal(raw, &c); err != nil {
		return ErrPushTokenInvalid
	}
	if i.now().Unix() >= c.Exp {
		return ErrPushTokenExpired
	}
	if action != pushActionPush || c.Act != pushActionPush || c.Repo != repo || c.Tag != tag {
		return ErrPushTokenScope
	}
	return nil
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

// DigestVerifier asks the control plane's registry whether a manifest exists.
type DigestVerifier interface {
	ManifestExists(ctx context.Context, repo, digest string) (bool, error)
}

// ErrDigestMissing means the builder reported a digest the registry does not hold.
var ErrDigestMissing = errors.New("build: reported image digest is not in the registry")

// VerifyPushedDigest fails closed: a malformed digest or a registry error is a failed build.
func VerifyPushedDigest(ctx context.Context, v DigestVerifier, ref, digest string) error {
	if !digestPattern.MatchString(digest) {
		return fmt.Errorf("build: builder reported a malformed digest %q", digest)
	}
	repo, _, err := SplitRef(ref)
	if err != nil {
		return err
	}
	ok, err := v.ManifestExists(ctx, repo, digest)
	if err != nil {
		return fmt.Errorf("build: verify pushed digest: %w", err)
	}
	if !ok {
		return ErrDigestMissing
	}
	return nil
}

// applyPush turns a solve's docker-tar export into a registry push by digest
// and attaches the job token as the only registry credential.
func applyPush(opt *bkclient.SolveOpt, g *PushGrant) error {
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
	token := g.Token
	opt.Session = append(opt.Session, authprovider.NewDockerAuthProvider(authprovider.DockerAuthProviderConfig{
		AuthConfigProvider: func(_ context.Context, h string, _ []string, _ authprovider.ExpireCachedAuthCheck) (types.AuthConfig, error) {
			if h != host {
				return types.AuthConfig{}, nil
			}
			return types.AuthConfig{Username: pushUser, Password: token}, nil
		},
	}))
	return nil
}
