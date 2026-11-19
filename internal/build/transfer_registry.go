package build

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

// RegistryCredentials returns the control plane's own registry login.
type RegistryCredentials func(ctx context.Context) (user, password string, err error)

// RegistryClient implements DigestVerifier with a manifest HEAD.
type RegistryClient struct {
	Insecure bool
	// Bearer, when set, supplies a scoped token for repo and wins over Creds (token-auth registry).
	Bearer func(repo string) (string, error)
	Creds  RegistryCredentials
	HTTP   *http.Client
}

const manifestAccept = "application/vnd.oci.image.manifest.v1+json, application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.v2+json, application/vnd.docker.distribution.manifest.list.v2+json"

// ManifestDigest asks the registry itself, so its answer wins over the builder's report.
func (c *RegistryClient) ManifestDigest(ctx context.Context, repo, reference string) (string, error) {
	host, path, ok := strings.Cut(repo, "/")
	if !ok || host == "" || path == "" || (c.Creds == nil && c.Bearer == nil) {
		return "", errors.New("build: registry client needs a host/repo and credentials")
	}
	scheme := "https"
	if c.Insecure {
		scheme = "http"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, scheme+"://"+host+"/v2/"+path+"/manifests/"+reference, nil)
	if err != nil {
		return "", err
	}
	if c.Bearer != nil {
		tok, err := c.Bearer(path)
		if err != nil || tok == "" {
			return "", errors.New("build: registry token is not available")
		}
		req.Header.Set("Authorization", "Bearer "+tok)
	} else {
		user, pass, err := c.Creds(ctx)
		if err != nil || user == "" || pass == "" {
			return "", errors.New("build: registry credentials are not available")
		}
		req.SetBasicAuth(user, pass)
	}
	req.Header.Set("Accept", manifestAccept)
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("build: registry manifest check: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		return resp.Header.Get("Docker-Content-Digest"), nil
	case http.StatusNotFound:
		return "", nil
	default:
		return "", fmt.Errorf("build: registry manifest check answered %d", resp.StatusCode)
	}
}

// ValidateInsecureHost allows plain HTTP only for loopback and private (RFC1918, ULA) addresses,
// which covers the mesh; the shared CGNAT range 100.64.0.0/10 needs an explicit opt-in.
func ValidateInsecureHost(host string, allowCGNAT bool) error {
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		h = host
	}
	if h == "localhost" {
		return nil
	}
	ip, err := netip.ParseAddr(h)
	if err != nil {
		return fmt.Errorf("build: insecure registry %q must be an IP address on a private network", host)
	}
	ip = ip.Unmap()
	if ip.IsLoopback() || ip.IsPrivate() {
		return nil
	}
	if allowCGNAT && netip.MustParsePrefix("100.64.0.0/10").Contains(ip) {
		return nil
	}
	return fmt.Errorf("build: insecure registry %q is not loopback or private", host)
}

// CertBundlePEM wraps the signing key in a self-signed certificate, the form the registry's trusted bundle takes.
func CertBundlePEM(key *ecdsa.PrivateKey, now time.Time, validFor time.Duration) ([]byte, error) {
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "levelrail-registry-token"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(validFor),
		KeyUsage: x509.KeyUsageDigitalSignature, BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("build: registry token certificate: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), nil
}

// RegistryAccess is one entry of a distribution token's access claim.
type RegistryAccess struct {
	Type    string   `json:"type"`
	Name    string   `json:"name"`
	Actions []string `json:"actions"`
}

// RegistryTokenService turns a job push token into a registry bearer token.
type RegistryTokenService struct {
	Issuer *PushTokenIssuer
	Key    *ecdsa.PrivateKey
	// Host returns the canonical registry host; scope names are relative to it.
	Host        func() string
	Service     string
	TokenIssuer string
	// BearerTTL is the live bearer lifetime; builds refresh through Exchange so revocation applies.
	BearerTTL func() time.Duration
	now       func() time.Time
}

// ErrRegistryScope is returned when the request asks for nothing the job token grants.
var ErrRegistryScope = errors.New("build: no requested scope is granted by this token")

// Exchange grants push and pull on the job's single repository only; pull-only
// requests and every other repository get an empty grant, never an error.
func (s *RegistryTokenService) Exchange(pushToken string, scopes []string) (string, time.Time, error) {
	if s.Issuer == nil || s.Key == nil || s.Host == nil || s.Host() == "" || s.ttl() <= 0 {
		return "", time.Time{}, errors.New("build: registry token service is not configured")
	}
	var access []RegistryAccess
	var tokExp time.Time
	for _, sc := range scopes {
		parts := strings.Split(sc, ":")
		if len(parts) != 3 || parts[0] != "repository" || parts[1] == "" {
			continue
		}
		exp, err := s.Issuer.VerifyRepo(pushToken, s.Host()+"/"+parts[1])
		if err != nil {
			if errors.Is(err, ErrPushTokenScope) {
				continue
			}
			return "", time.Time{}, err
		}
		if !hasAction(parts[2], pushActionPush) {
			continue
		}
		tokExp = exp
		access = append(access, RegistryAccess{Type: "repository", Name: parts[1], Actions: []string{"push", "pull"}})
	}
	if len(access) == 0 {
		// A valid token that asks for nothing grantable still has to prove itself.
		if _, err := s.Issuer.parse(pushToken); err != nil {
			return "", time.Time{}, err
		}
		return s.sign(nil, s.nowT().Add(s.ttl()))
	}
	exp := s.nowT().Add(s.ttl())
	if tokExp.Before(exp) {
		exp = tokExp
	}
	return s.sign(access, exp)
}

// MintPull gives the control plane a short pull-only token for one repository.
func (s *RegistryTokenService) MintPull(name string) (string, error) {
	if s.Key == nil || name == "" || s.ttl() <= 0 {
		return "", errors.New("build: registry token service is not configured")
	}
	tok, _, err := s.sign([]RegistryAccess{{Type: "repository", Name: name, Actions: []string{"pull"}}}, s.nowT().Add(s.ttl()))
	return tok, err
}

func (s *RegistryTokenService) ttl() time.Duration {
	if s.BearerTTL == nil {
		return 0
	}
	return s.BearerTTL()
}

func hasAction(list, want string) bool {
	for _, a := range strings.Split(list, ",") {
		if a == want {
			return true
		}
	}
	return false
}

func (s *RegistryTokenService) nowT() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *RegistryTokenService) sign(access []RegistryAccess, exp time.Time) (string, time.Time, error) {
	kid, err := LibtrustKeyID(&s.Key.PublicKey)
	if err != nil {
		return "", time.Time{}, err
	}
	jti := make([]byte, 12)
	if _, err := rand.Read(jti); err != nil {
		return "", time.Time{}, err
	}
	now := s.nowT()
	if access == nil {
		access = []RegistryAccess{}
	}
	hdr, _ := json.Marshal(map[string]string{"typ": "JWT", "alg": "ES256", "kid": kid})
	claims, _ := json.Marshal(map[string]any{
		"iss": s.TokenIssuer, "sub": pushUser, "aud": s.Service,
		"exp": exp.Unix(), "nbf": now.Add(-10 * time.Second).Unix(), "iat": now.Unix(),
		"jti": base64.RawURLEncoding.EncodeToString(jti), "access": access,
	})
	signing := base64.RawURLEncoding.EncodeToString(hdr) + "." + base64.RawURLEncoding.EncodeToString(claims)
	sum := sha256.Sum256([]byte(signing))
	r, sg, err := ecdsa.Sign(rand.Reader, s.Key, sum[:])
	if err != nil {
		return "", time.Time{}, err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	sg.FillBytes(sig[32:])
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig), exp, nil
}

// LibtrustKeyID is the key id format the registry matches against its trusted bundle.
func LibtrustKeyID(pub *ecdsa.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(der)
	enc := base32.StdEncoding.EncodeToString(sum[:30])
	var groups []string
	for i := 0; i < len(enc); i += 4 {
		groups = append(groups, enc[i:i+4])
	}
	return strings.Join(groups, ":"), nil
}

// ParseECKeyPEM reads a P-256 private key for token signing.
func ParseECKeyPEM(der []byte) (*ecdsa.PrivateKey, error) {
	k, err := x509.ParseECPrivateKey(der)
	if err != nil {
		return nil, errors.New("build: registry token key is not an EC private key")
	}
	if k.Curve != elliptic.P256() {
		return nil, errors.New("build: registry token key must be P-256")
	}
	return k, nil
}
