package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

const (
	envCIOIDCClockSkew       = "APP_CI_OIDC_CLOCK_SKEW"
	envCIOIDCMaxTokenAge     = "APP_CI_OIDC_MAX_TOKEN_AGE" //nolint:gosec // env var name, not a credential
	envCIOIDCJWKSTimeout     = "APP_CI_OIDC_JWKS_TIMEOUT"
	envCIOIDCJWKSMaxBytes    = "APP_CI_OIDC_JWKS_MAX_BYTES"
	envCIOIDCJWKSMinRefresh  = "APP_CI_OIDC_JWKS_MIN_REFRESH"
	envCIOIDCJWKSTTL         = "APP_CI_OIDC_JWKS_TTL"
	maxCIJWKSRedirects       = 3
	defaultCIOIDCMaxTokenAge = time.Hour
)

var (
	ciSigningAlgs = []jose.SignatureAlgorithm{jose.RS256, jose.ES256}
	// CGNAT, IETF protocol, benchmarking, reserved, NAT64 and 6to4: the last two can wrap a private IPv4 address.
	ciBlockedPrefixes = []netip.Prefix{
		netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("198.18.0.0/15"),
		netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("64:ff9b::/96"), netip.MustParsePrefix("64:ff9b:1::/48"),
		netip.MustParsePrefix("2002::/16"),
	}
	errCIJWKSBlockedAddr = errors.New("api: jwks host resolves to a private, loopback or link-local address")
	errCIJWKSRedirect    = errors.New("api: jwks redirect to another host or scheme refused")
	errCIOIDCUnknownKey  = errors.New("api: ci oidc token signed with an unknown key")
)

// ciOIDCConfig bounds token validation and JWKS fetching for the GitLab and CircleCI verifiers.
type ciOIDCConfig struct {
	ClockSkew    time.Duration
	MaxTokenAge  time.Duration
	FetchTimeout time.Duration
	MaxJWKSBytes int64
	MinRefresh   time.Duration
	KeysTTL      time.Duration
}

func ciOIDCConfigFromEnv() ciOIDCConfig {
	return ciOIDCConfig{
		ClockSkew:    envDuration(envCIOIDCClockSkew, time.Minute),
		MaxTokenAge:  ciMaxTokenAgeFromEnv(),
		FetchTimeout: envDuration(envCIOIDCJWKSTimeout, 10*time.Second),
		MaxJWKSBytes: int64(envInt(envCIOIDCJWKSMaxBytes, 256<<10)),
		MinRefresh:   envDuration(envCIOIDCJWKSMinRefresh, 5*time.Minute),
		KeysTTL:      envDuration(envCIOIDCJWKSTTL, time.Hour),
	}
}

// ciProviderSpec names the issuer a token must carry and where its keys live; both come from an owner-configured trust.
type ciProviderSpec struct {
	Provider string
	Issuer   string
	JWKSURL  string
}

// ciProviderVerifier verifies a token for an owner-configured issuer. A test verifier set with SetCIIDTokenVerifier may implement it.
type ciProviderVerifier interface {
	VerifyProvider(ctx context.Context, rawToken string, spec ciProviderSpec, audience string) (CIClaims, error)
}

type ciKeySet struct {
	mu        sync.Mutex
	keys      map[string]jose.JSONWebKey
	fetchedAt time.Time
	attemptAt time.Time
}

// ciJWTVerifier checks RS256 and ES256 tokens against a JWKS it fetches through an SSRF-guarded client.
type ciJWTVerifier struct {
	cfg    ciOIDCConfig
	now    func() time.Time
	client *http.Client
	mu     sync.Mutex
	sets   map[string]*ciKeySet
}

func newCIJWTVerifier(cfg ciOIDCConfig, allowAddr func(netip.Addr) bool) *ciJWTVerifier {
	cfg.KeysTTL = max(cfg.KeysTTL, cfg.MinRefresh)
	return &ciJWTVerifier{cfg: cfg, now: time.Now, client: newCIJWKSClient(cfg, allowAddr), sets: map[string]*ciKeySet{}}
}

var defaultCIJWTVerifier = sync.OnceValue(func() *ciJWTVerifier {
	return newCIJWTVerifier(ciOIDCConfigFromEnv(), ciPublicAddr)
})

// ciPublicAddr reports whether a resolved JWKS address is on the public internet.
func ciPublicAddr(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsValid() && !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast() && !ip.IsInterfaceLocalMulticast() && !ip.IsMulticast() &&
		!ip.IsUnspecified() && !slices.ContainsFunc(ciBlockedPrefixes, func(p netip.Prefix) bool { return p.Contains(ip) }) &&
		(!ip.Is4() || ip.As4()[0] != 0)
}

// ciMaxTokenAgeFromEnv defaults to an hour and fails closed: only an explicit 0 or off disables the age check.
func ciMaxTokenAgeFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv(envCIOIDCMaxTokenAge))
	if raw == "0" || strings.EqualFold(raw, "off") {
		return 0
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d
	}
	return defaultCIOIDCMaxTokenAge
}

// newCIJWKSClient checks the address after DNS resolution, so a rebinding hostname cannot reach the host network.
func newCIJWKSClient(cfg ciOIDCConfig, allowAddr func(netip.Addr) bool) *http.Client {
	dialer := &net.Dialer{Timeout: cfg.FetchTimeout, Control: func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return fmt.Errorf("api: jwks dial address: %w", err)
		}
		ip, err := netip.ParseAddr(host)
		if err != nil || !allowAddr(ip) {
			return errCIJWKSBlockedAddr
		}
		return nil
	}}
	tr := &http.Transport{
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   cfg.FetchTimeout,
		ResponseHeaderTimeout: cfg.FetchTimeout,
		MaxIdleConns:          4,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	return &http.Client{Transport: tr, Timeout: cfg.FetchTimeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxCIJWKSRedirects || req.URL.Scheme != "https" || req.URL.Host != via[0].URL.Host {
			return errCIJWKSRedirect
		}
		return nil
	}}
}

// VerifyProvider checks algorithm, kid, signature, issuer, audience and time claims, then maps the provider's claims.
func (v *ciJWTVerifier) VerifyProvider(ctx context.Context, rawToken string, spec ciProviderSpec, audience string) (CIClaims, error) {
	if audience == "" || spec.Issuer == "" || spec.JWKSURL == "" {
		return CIClaims{}, errors.New("api: ci oidc verifier needs an issuer, a jwks url and an audience")
	}
	tok, err := jwt.ParseSigned(rawToken, ciSigningAlgs)
	if err != nil {
		return CIClaims{}, fmt.Errorf("api: parse ci oidc token: %w", err)
	}
	if len(tok.Headers) != 1 || tok.Headers[0].KeyID == "" {
		return CIClaims{}, errors.New("api: ci oidc token has no kid")
	}
	hdr := tok.Headers[0]
	key, err := v.key(ctx, spec.JWKSURL, hdr.KeyID)
	if err != nil {
		return CIClaims{}, err
	}
	if key.Algorithm != "" && key.Algorithm != hdr.Algorithm {
		return CIClaims{}, errors.New("api: ci oidc token algorithm does not match its key")
	}
	var std jwt.Claims
	var all map[string]json.RawMessage
	if err := tok.Claims(key.Key, &std, &all); err != nil {
		return CIClaims{}, fmt.Errorf("api: verify ci oidc signature: %w", err)
	}
	if err := v.checkStandard(std, spec.Issuer, audience); err != nil {
		return CIClaims{}, err
	}
	return mapCIClaims(spec, std.Subject, all)
}

func (v *ciJWTVerifier) checkStandard(std jwt.Claims, issuer, audience string) error {
	now, skew := v.now(), v.cfg.ClockSkew
	switch {
	case std.Issuer != issuer:
		return errors.New("api: ci oidc token issuer is not the trusted issuer")
	case !std.Audience.Contains(audience):
		return errors.New("api: ci oidc token audience is not this control plane")
	case std.Expiry == nil:
		return errors.New("api: ci oidc token has no exp")
	case now.After(std.Expiry.Time().Add(skew)):
		return errors.New("api: ci oidc token expired")
	case std.NotBefore != nil && now.Add(skew).Before(std.NotBefore.Time()):
		return errors.New("api: ci oidc token not valid yet")
	case std.IssuedAt != nil && now.Add(skew).Before(std.IssuedAt.Time()):
		return errors.New("api: ci oidc token issued in the future")
	}
	if v.cfg.MaxTokenAge > 0 && (std.IssuedAt == nil || now.Sub(std.IssuedAt.Time()) > v.cfg.MaxTokenAge+skew) {
		return errors.New("api: ci oidc token is older than the allowed age")
	}
	return nil
}

func (v *ciJWTVerifier) set(jwksURL string) *ciKeySet {
	v.mu.Lock()
	defer v.mu.Unlock()
	s, ok := v.sets[jwksURL]
	if !ok {
		s = &ciKeySet{}
		v.sets[jwksURL] = s
	}
	return s
}

// key refetches on an unknown kid at most once per MinRefresh, so forged kids cannot turn the exchange into a fetch amplifier.
func (v *ciJWTVerifier) key(ctx context.Context, jwksURL, kid string) (jose.JSONWebKey, error) {
	s := v.set(jwksURL)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := v.now()
	if k, ok := s.keys[kid]; ok && now.Sub(s.fetchedAt) < v.cfg.KeysTTL {
		return k, nil
	}
	if !s.attemptAt.IsZero() && now.Sub(s.attemptAt) < v.cfg.MinRefresh {
		return jose.JSONWebKey{}, errCIOIDCUnknownKey
	}
	s.attemptAt = now
	keys, err := v.fetch(ctx, jwksURL)
	if err != nil {
		return jose.JSONWebKey{}, err
	}
	s.keys, s.fetchedAt = keys, now
	k, ok := keys[kid]
	if !ok {
		return jose.JSONWebKey{}, errCIOIDCUnknownKey
	}
	return k, nil
}

func (v *ciJWTVerifier) fetch(ctx context.Context, rawURL string) (map[string]jose.JSONWebKey, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return nil, errors.New("api: jwks url must be a plain https url")
	}
	ctx, cancel := context.WithTimeout(ctx, v.cfg.FetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("api: build jwks request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("api: fetch jwks: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api: jwks answered %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, v.cfg.MaxJWKSBytes+1))
	if err != nil {
		return nil, fmt.Errorf("api: read jwks: %w", err)
	}
	if int64(len(body)) > v.cfg.MaxJWKSBytes {
		return nil, errors.New("api: jwks is larger than the allowed size")
	}
	return parseCIJWKS(body)
}

// parseCIJWKS keeps public signing keys with a kid; one unsupported key type must not drop the whole set.
func parseCIJWKS(body []byte) (map[string]jose.JSONWebKey, error) {
	var doc struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("api: decode jwks: %w", err)
	}
	out := make(map[string]jose.JSONWebKey, len(doc.Keys))
	for _, raw := range doc.Keys {
		var k jose.JSONWebKey
		if err := k.UnmarshalJSON(raw); err != nil {
			continue
		}
		if k.KeyID == "" || !k.Valid() || !k.IsPublic() || (k.Use != "" && k.Use != "sig") {
			continue
		}
		if k.Algorithm != "" && k.Algorithm != string(jose.RS256) && k.Algorithm != string(jose.ES256) {
			continue
		}
		out[k.KeyID] = k
	}
	if len(out) == 0 {
		return nil, errors.New("api: jwks has no usable signing keys")
	}
	return out, nil
}

// unverifiedCIIssuer reads iss only to pick a trusted issuer; nothing is fetched unless it equals a configured one.
func unverifiedCIIssuer(rawToken string) string {
	tok, err := jwt.ParseSigned(rawToken, ciSigningAlgs)
	if err != nil {
		return ""
	}
	var c struct {
		Issuer string `json:"iss"`
	}
	if err := tok.UnsafeClaimsWithoutVerification(&c); err != nil {
		return ""
	}
	return c.Issuer
}

// ciLogSubject is the only claim value exchange logs carry: printable and clipped.
func ciLogSubject(s string) string {
	return clipClaim(strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return -1
	}, s))
}
