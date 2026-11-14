package cloudconn

import (
	"context"
	"crypto"
	"crypto/md5" //nolint:gosec // the OCI key fingerprint is defined as an MD5 digest
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretsource"
)

// File names inside a materialized OCI credential directory.
const (
	OracleConfigFileName = "config"
	OracleKeyFileName    = "oci_api_key.pem" //nolint:gosec // a file name, not a credential
	oracleProfile        = "DEFAULT"
)

var (
	ocidRe        = regexp.MustCompile(`^ocid1\.([a-z]+)\.oc[0-9]+\.[a-z0-9-]*\.[a-z0-9]{8,}$`)
	ociRegionRe   = regexp.MustCompile(`^[a-z]{2,}(-[a-z]+)+-[0-9]+$`)
	fingerprintRe = regexp.MustCompile(`^([0-9a-fA-F]{2}:){15}[0-9a-fA-F]{2}$`)
	policyNameRe  = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
	namespaceRe   = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
)

func envIntDefault(name string, def int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name))); err == nil && v > 0 {
		return v
	}
	return def
}

// Bounds read per call so an operator can tighten them without a rebuild.
func oracleMinRSABits() int { return envIntDefault("APP_CLOUD_OCI_MIN_RSA_BITS", 2048) }
func oracleMaxSecretBytes() int {
	return envIntDefault("APP_CLOUD_OCI_MAX_SECRET_BYTES", 16384)
}
func oracleCheckTimeout() time.Duration {
	return time.Duration(envIntDefault("APP_CLOUD_CHECK_TIMEOUT_SECONDS", 15)) * time.Second
}

// OracleSecret is the sealed half of an OCI connection: the user, the key fingerprint
// and the API signing key. Every formatter masks the key.
type OracleSecret struct {
	UserOCID    string
	Fingerprint string
	key         []byte
}

// NewOracleSecret copies the PEM so the caller can wipe its own buffer.
func NewOracleSecret(userOCID, fingerprint string, pemKey []byte) OracleSecret {
	return OracleSecret{UserOCID: userOCID, Fingerprint: fingerprint, key: append([]byte(nil), pemKey...)}
}

// KeyPEM returns a copy of the key for one upload; the caller wipes it after use.
func (s OracleSecret) KeyPEM() []byte { return append([]byte(nil), s.key...) }

// String masks the key.
func (s OracleSecret) String() string {
	return fmt.Sprintf("OracleSecret{user=%s fingerprint=%s key=%s}", s.UserOCID, s.Fingerprint, secretsource.Mask)
}

// GoString masks the key for %#v.
func (s OracleSecret) GoString() string { return s.String() }

// MarshalJSON keeps the key out of any API or log encoding of the struct.
func (s OracleSecret) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{"user_ocid": s.UserOCID, "fingerprint": s.Fingerprint, "private_key": secretsource.Mask})
}

type oracleSealed struct {
	User        string `json:"user"`
	Fingerprint string `json:"fingerprint"`
	Key         string `json:"private_key"`
}

// ParseOracleSecret decodes a resolved sealed bundle. Errors never echo the input.
func ParseOracleSecret(v secretsource.Value) (OracleSecret, error) {
	if v.Len() == 0 || v.Len() > oracleMaxSecretBytes() {
		return OracleSecret{}, invalid("oracle credential bundle is empty or too large")
	}
	var b oracleSealed
	if err := json.Unmarshal(v.Reveal(), &b); err != nil {
		return OracleSecret{}, invalid("oracle credential bundle is not valid")
	}
	return NewOracleSecret(b.User, b.Fingerprint, []byte(b.Key)), nil
}

// SealedSetter stores a value sealed at rest; secrets.Manager satisfies it.
type SealedSetter interface {
	SetValue(ctx context.Context, namespace, key, plaintext string) error
}

// SealOracleSecret validates the secret, stores it sealed and returns the NAMESPACE/KEY
// reference a connection keeps. The key never reaches the connection row.
func SealOracleSecret(ctx context.Context, s SealedSetter, namespace, key string, sec OracleSecret) (string, error) {
	if s == nil {
		return "", invalid("no sealed secret store")
	}
	if !namespaceRe.MatchString(namespace) || !namespaceRe.MatchString(key) {
		return "", invalid("namespace and key must be short names")
	}
	if _, err := validateOracleSecret(sec); err != nil {
		return "", err
	}
	raw, err := json.Marshal(oracleSealed{User: sec.UserOCID, Fingerprint: sec.Fingerprint, Key: string(sec.key)})
	if err != nil {
		return "", invalid("oracle credential could not be encoded")
	}
	if err := s.SetValue(ctx, namespace, key, string(raw)); err != nil {
		return "", errors.New("cloudconn: sealing the oracle credential failed")
	}
	return namespace + "/" + key, nil
}

// OracleCredential is everything one OCI step needs. Tenancy, region and compartment
// come from the connection settings; IaC documents only name the connection.
type OracleCredential struct {
	TenancyOCID     string
	CompartmentOCID string
	Region          string
	Secret          OracleSecret
}

// OracleFromTarget joins the stored settings with the resolved sealed bundle.
func OracleFromTarget(t Target) (OracleCredential, error) {
	sec, err := ParseOracleSecret(t.Credential)
	if err != nil {
		return OracleCredential{}, err
	}
	c := t.Connection.Config
	return OracleCredential{TenancyOCID: c.AccountID, CompartmentOCID: c.Compartment, Region: c.Region, Secret: sec}, nil
}

func checkOCID(field, want, v string) error {
	m := ocidRe.FindStringSubmatch(v)
	if len(v) > 255 || m == nil || m[1] != want {
		return invalid("%s must be an OCI %s OCID", field, want)
	}
	return nil
}

func validateOracleSettings(tenancy, compartment, region string) error {
	if err := checkOCID("tenancy", "tenancy", tenancy); err != nil {
		return err
	}
	if err := checkOCID("compartment", "compartment", compartment); err != nil {
		return err
	}
	if !ociRegionRe.MatchString(region) || len(region) > 64 {
		return invalid("region must look like us-phoenix-1")
	}
	return nil
}

// ValidateOracle checks every field offline: OCID shapes, region, fingerprint and the key.
func ValidateOracle(c OracleCredential) error {
	if err := validateOracleSettings(c.TenancyOCID, c.CompartmentOCID, c.Region); err != nil {
		return err
	}
	_, err := validateOracleSecret(c.Secret)
	return err
}

func validateOracleSecret(s OracleSecret) (*rsa.PrivateKey, error) {
	if err := checkOCID("user", "user", s.UserOCID); err != nil {
		return nil, err
	}
	if !fingerprintRe.MatchString(s.Fingerprint) {
		return nil, invalid("fingerprint must be 16 colon separated hex pairs")
	}
	key, err := parseOracleKey(s.key)
	if err != nil {
		return nil, err
	}
	fp, err := oracleFingerprint(&key.PublicKey)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(fp, s.Fingerprint) {
		return nil, invalid("fingerprint does not match the private key")
	}
	return key, nil
}

func parseOracleKey(raw []byte) (*rsa.PrivateKey, error) {
	if len(raw) == 0 || len(raw) > oracleMaxSecretBytes() {
		return nil, invalid("private key is empty or too large")
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, invalid("private key is not PEM encoded")
	}
	// Passphrases are not accepted: the key is already sealed at rest, and a passphrase would be a second secret to store.
	if block.Type == "ENCRYPTED PRIVATE KEY" || strings.Contains(block.Headers["Proc-Type"], "ENCRYPTED") {
		return nil, invalid("private key is encrypted; export it without a passphrase, it is sealed at rest")
	}
	var key *rsa.PrivateKey
	switch block.Type {
	case "RSA PRIVATE KEY":
		k, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, invalid("private key could not be parsed")
		}
		key = k
	case "PRIVATE KEY":
		k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, invalid("private key could not be parsed")
		}
		rk, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, invalid("private key must be RSA")
		}
		key = rk
	default:
		return nil, invalid("private key must be an RSA PEM key")
	}
	if key.N.BitLen() < oracleMinRSABits() {
		return nil, invalid("private key must be at least %d bits", oracleMinRSABits())
	}
	return key, nil
}

func oracleFingerprint(pub *rsa.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", invalid("public key could not be encoded")
	}
	sum := md5.Sum(der) //nolint:gosec // OCI defines the fingerprint this way, it is not a security check
	parts := make([]string, len(sum))
	for i, b := range sum {
		parts[i] = fmt.Sprintf("%02x", b)
	}
	return strings.Join(parts, ":"), nil
}

// OracleCredFiles is a materialized per-step credential. It exposes paths only.
type OracleCredFiles struct {
	dir string
	red *secretsource.Redactor
}

// Dir is the host directory holding config and key; mount it read-only into the step.
func (h *OracleCredFiles) Dir() string { return h.dir }

// ConfigPath is the host path of the OCI config file.
func (h *OracleCredFiles) ConfigPath() string { return filepath.Join(h.dir, OracleConfigFileName) }

// Redact scrubs the key material, whole or line by line, from a log line.
func (h *OracleCredFiles) Redact(line string) string { return h.red.Mask(line) }

// Close removes the directory; safe to call twice.
func (h *OracleCredFiles) Close() error {
	if h.dir == "" {
		return nil
	}
	err := os.RemoveAll(h.dir)
	h.dir = ""
	return err
}

// MaterializeOracle writes an OCI config profile and key into a fresh 0700 directory under
// base (a tmpfs). mountDir is where the step sees that directory, so key_file resolves there.
func MaterializeOracle(base, mountDir string, c OracleCredential) (*OracleCredFiles, error) {
	if err := ValidateOracle(c); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(base) {
		return nil, invalid("credential base must be an absolute path")
	}
	if mountDir != "" && (!filepath.IsAbs(mountDir) || strings.ContainsAny(mountDir, "\x00\r\n=[]")) {
		return nil, invalid("credential mount dir must be a plain absolute path")
	}
	dir, err := os.MkdirTemp(base, "oci-")
	if err != nil {
		return nil, fmt.Errorf("cloudconn: credential dir: %w", err)
	}
	// MkdirTemp creates the directory 0700, so only this process user can enter it.
	h := &OracleCredFiles{dir: dir, red: oracleRedactor(c.Secret.key)}
	if mountDir == "" {
		mountDir = dir
	}
	cfg := fmt.Sprintf("[%s]\nuser=%s\nfingerprint=%s\ntenancy=%s\nregion=%s\nkey_file=%s\n",
		oracleProfile, c.Secret.UserOCID, strings.ToLower(c.Secret.Fingerprint), c.TenancyOCID, c.Region,
		filepath.Join(mountDir, OracleKeyFileName))
	if err := writePrivate(filepath.Join(dir, OracleKeyFileName), c.Secret.key); err != nil {
		_ = h.Close()
		return nil, err
	}
	if err := writePrivate(h.ConfigPath(), []byte(cfg)); err != nil {
		_ = h.Close()
		return nil, err
	}
	return h, nil
}

// WithOracleCredential materializes the credential for fn and always removes it afterwards.
func WithOracleCredential(base, mountDir string, c OracleCredential, fn func(*OracleCredFiles) error) (err error) {
	h, err := MaterializeOracle(base, mountDir, c)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := h.Close(); err == nil && cerr != nil {
			err = fmt.Errorf("cloudconn: credential cleanup: %w", cerr)
		}
	}()
	return fn(h)
}

func writePrivate(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // path is inside a dir this package just created
	if err != nil {
		return fmt.Errorf("cloudconn: credential file: %w", err)
	}
	_, werr := f.Write(data)
	cerr := f.Close()
	if werr != nil || cerr != nil {
		return errors.New("cloudconn: credential file could not be written")
	}
	return os.Chmod(path, 0o600)
}

// oracleRedactor masks the whole PEM and each body line, since logs may wrap or split it.
func oracleRedactor(key []byte) *secretsource.Redactor {
	r := secretsource.NewRedactor()
	r.Add(key)
	r.Add([]byte(strings.TrimSpace(string(key))))
	for _, ln := range strings.Split(string(key), "\n") {
		ln = strings.TrimSpace(ln)
		if len(ln) >= 16 && !strings.HasPrefix(ln, "-----") {
			r.Add([]byte(ln))
		}
	}
	return r
}

// oracleSigningString builds the string the OCI HTTP signature covers for a body-less request.
// Header order follows the published example: date, (request-target), host.
func oracleSigningString(req *http.Request) string {
	target := req.URL.EscapedPath()
	if req.URL.RawQuery != "" {
		target += "?" + req.URL.RawQuery
	}
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	return "date: " + req.Header.Get("Date") + "\n" +
		"(request-target): " + strings.ToLower(req.Method) + " " + target + "\n" +
		"host: " + host
}

// signOracleRequest adds Date and Authorization. Only GET is signed: this package is read-only.
func signOracleRequest(req *http.Request, keyID string, key *rsa.PrivateKey, now time.Time) error {
	if req.Method != http.MethodGet {
		return errors.New("cloudconn: only read-only GET requests are signed")
	}
	if req.Header.Get("Date") == "" {
		req.Header.Set("Date", now.UTC().Format(http.TimeFormat))
	}
	digest := sha256.Sum256([]byte(oracleSigningString(req)))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return errors.New("cloudconn: request signing failed")
	}
	req.Header.Set("Authorization", fmt.Sprintf(
		`Signature version="1",keyId="%s",algorithm="rsa-sha256",headers="date (request-target) host",signature="%s"`,
		keyID, base64.StdEncoding.EncodeToString(sig)))
	return nil
}

// HealthChecker makes one read-only call with an OCI credential and returns a Class constant.
type HealthChecker interface {
	Check(ctx context.Context, c OracleCredential) (string, error)
}

// OracleHTTPChecker lists at most one instance in the compartment, which the generated
// compartment policy allows, so a pass proves the key and the scope together.
type OracleHTTPChecker struct {
	Client  *http.Client
	BaseURL func(region string) string
	Now     func() time.Time
}

// NewOracleHTTPChecker uses the public regional compute endpoint and an env bounded timeout.
func NewOracleHTTPChecker() OracleHTTPChecker {
	return OracleHTTPChecker{
		Client:  &http.Client{Timeout: oracleCheckTimeout()},
		BaseURL: func(region string) string { return "https://iaas." + region + ".oraclecloud.com" },
		Now:     time.Now,
	}
}

// Check signs and sends one GET. Provider output never leaves this function.
func (o OracleHTTPChecker) Check(ctx context.Context, c OracleCredential) (string, error) {
	if err := validateOracleSettings(c.TenancyOCID, c.CompartmentOCID, c.Region); err != nil {
		return ClassConfigInvalid, err
	}
	key, err := validateOracleSecret(c.Secret)
	if err != nil {
		return ClassConfigInvalid, err
	}
	if o.Client == nil || o.BaseURL == nil {
		return ClassUnknown, errors.New("cloudconn: oracle checker not configured")
	}
	now := time.Now
	if o.Now != nil {
		now = o.Now
	}
	u := o.BaseURL(c.Region) + "/20160918/instances?compartmentId=" + url.QueryEscape(c.CompartmentOCID) + "&limit=1"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return ClassConfigInvalid, errors.New("cloudconn: oracle endpoint is not valid")
	}
	keyID := c.TenancyOCID + "/" + c.Secret.UserOCID + "/" + strings.ToLower(c.Secret.Fingerprint)
	if err := signOracleRequest(req, keyID, key, now()); err != nil {
		return ClassUnknown, err
	}
	resp, err := o.Client.Do(req)
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) || errors.Is(err, context.DeadlineExceeded) {
			return ClassUnreachable, errors.New("cloudconn: oracle endpoint unreachable")
		}
		return ClassUnreachable, errors.New("cloudconn: oracle request failed")
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return ClassOK, nil
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
		return ClassAuthFailed, nil
	case http.StatusBadRequest:
		return ClassConfigInvalid, nil
	default:
		return ClassUnknown, nil
	}
}

// OracleProvider is the read-only OCI side. It is not in DefaultRegistry; wiring it is opt-in.
type OracleProvider struct {
	Checker HealthChecker
}

// NewOracleProvider returns a provider backed by the real signed HTTP checker.
func NewOracleProvider() OracleProvider { return OracleProvider{Checker: NewOracleHTTPChecker()} }

// Name returns the provider name.
func (OracleProvider) Name() string { return ProviderOracle }

// ValidateConfig applies the shared rules, then the OCI shapes.
func (OracleProvider) ValidateConfig(cfg Config, authMethod string) error {
	if err := validateProviderConfig(ProviderOracle, cfg, authMethod); err != nil {
		return err
	}
	return validateOracleSettings(cfg.AccountID, cfg.Compartment, cfg.Region)
}

// Check resolves the sealed bundle from the target and runs one read-only call.
func (p OracleProvider) Check(ctx context.Context, t Target) (string, error) {
	if t.Connection.AuthMethod != AuthSecretRef && t.Connection.AuthMethod != AuthStatic {
		return ClassUnsupported, fmt.Errorf("%w: oracle %s health check", ErrUnsupported, t.Connection.AuthMethod)
	}
	if t.Credential.Len() == 0 {
		return ClassCredentialUnresolved, errors.New("cloudconn: no oracle credential")
	}
	c, err := OracleFromTarget(t)
	if err != nil {
		return ClassConfigInvalid, err
	}
	if p.Checker == nil {
		return ClassUnknown, errors.New("cloudconn: oracle checker not configured")
	}
	return p.Checker.Check(ctx, c)
}

// ListRegions needs a tenancy wide read, which the compartment policy does not grant.
func (OracleProvider) ListRegions(context.Context, Target) ([]string, error) {
	return nil, fmt.Errorf("%w: oracle regions", ErrUnsupported)
}

// PolicyStatements returns the least privilege OCI IAM policy the user applies themselves.
// Every statement is scoped to one compartment; Levelrail never creates IAM.
func PolicyStatements(compartment, group string) ([]string, error) {
	if !policyNameRe.MatchString(group) {
		return nil, invalid("group must be a plain OCI group name")
	}
	var scope string
	switch {
	case ocidRe.MatchString(compartment):
		if err := checkOCID("compartment", "compartment", compartment); err != nil {
			return nil, err
		}
		scope = "compartment id " + compartment
	case policyNameRe.MatchString(compartment) && !strings.EqualFold(compartment, "tenancy") && !strings.EqualFold(compartment, "root"):
		scope = "compartment " + compartment
	default:
		return nil, invalid("compartment must be a compartment OCID or name, never the tenancy")
	}
	out := make([]string, 0, 3)
	for _, res := range []string{"instance-family", "virtual-network-family", "buckets"} {
		out = append(out, fmt.Sprintf("Allow group %s to manage %s in %s", group, res, scope))
	}
	return out, nil
}

// SealedReader reads back a value SealOracleSecret stored; secrets.Manager satisfies it.
type SealedReader interface {
	Resolve(ctx context.Context, namespace, key string) (string, error)
}

// OpenOracleSealed opens the sealed bundle a static oracle connection references.
// The returned error never carries the bundle or the reference value.
func OpenOracleSealed(ctx context.Context, r SealedReader, c Connection) (OracleCredential, error) {
	if r == nil {
		return OracleCredential{}, invalid("no sealed secret store")
	}
	if c.Provider != ProviderOracle || c.AuthMethod != AuthStatic {
		return OracleCredential{}, invalid("connection is not a static oracle connection")
	}
	ns, key, ok := strings.Cut(c.CredentialRef, "/")
	if !ok || !namespaceRe.MatchString(ns) || !namespaceRe.MatchString(key) {
		return OracleCredential{}, invalid("connection has no sealed credential reference")
	}
	raw, err := r.Resolve(ctx, ns, key)
	if err != nil {
		return OracleCredential{}, errors.New("cloudconn: sealed oracle credential could not be read")
	}
	return OracleFromTarget(Target{Connection: c, Credential: secretsource.NewValue([]byte(raw), "")})
}
