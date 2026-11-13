package cloudconn

import (
	"bytes"
	"context"
	"crypto"
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
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretsource"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	tTenancy     = "ocid1.tenancy.oc1..aaaaaaaatenancyunique01"
	tUser        = "ocid1.user.oc1..aaaaaaaauserunique0001"
	tCompartment = "ocid1.compartment.oc1..aaaaaaaacompartment01"
)

var (
	keyOnce sync.Once
	tKey    *rsa.PrivateKey
	tPEM    []byte
)

func testKey(t *testing.T) (*rsa.PrivateKey, []byte, string) {
	t.Helper()
	keyOnce.Do(func() {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		tKey = k
		tPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)})
	})
	fp, err := oracleFingerprint(&tKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return tKey, tPEM, fp
}

// canaryLine is one base64 body line of the PEM, unique per test run.
func canaryLine(p []byte) string { return strings.Split(string(p), "\n")[1] }

func testCred(t *testing.T) OracleCredential {
	_, p, fp := testKey(t)
	return OracleCredential{TenancyOCID: tTenancy, CompartmentOCID: tCompartment, Region: "us-phoenix-1",
		Secret: NewOracleSecret(tUser, fp, p)}
}

func TestValidateOracleTable(t *testing.T) {
	small, _ := rsa.GenerateKey(rand.Reader, 1024) //nolint:gosec // undersized on purpose, the validator must reject it
	smallPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(small)})
	smallFP, _ := oracleFingerprint(&small.PublicKey)
	_, p, _ := testKey(t)
	pk8, _ := x509.MarshalPKCS8PrivateKey(tKey)
	pk8PEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pk8})
	encPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Headers: map[string]string{"Proc-Type": "4,ENCRYPTED", "DEK-Info": "AES-128-CBC,00"}, Bytes: []byte{1, 2}})
	enc8PEM := pem.EncodeToMemory(&pem.Block{Type: "ENCRYPTED PRIVATE KEY", Bytes: []byte{1, 2}})

	cases := map[string]struct {
		mut func(c *OracleCredential)
		ok  bool
	}{
		"valid pkcs1":         {func(*OracleCredential) {}, true},
		"valid pkcs8":         {func(c *OracleCredential) { c.Secret.key = pk8PEM }, true},
		"uppercase fp":        {func(c *OracleCredential) { c.Secret.Fingerprint = strings.ToUpper(c.Secret.Fingerprint) }, true},
		"tenancy wrong type":  {func(c *OracleCredential) { c.TenancyOCID = tUser }, false},
		"tenancy junk":        {func(c *OracleCredential) { c.TenancyOCID = "ocid1.tenancy.x" }, false},
		"compartment is root": {func(c *OracleCredential) { c.CompartmentOCID = tTenancy }, false},
		"user wrong type":     {func(c *OracleCredential) { c.Secret.UserOCID = tCompartment }, false},
		"region bad":          {func(c *OracleCredential) { c.Region = "Phoenix" }, false},
		"region newline":      {func(c *OracleCredential) { c.Region = "us-phoenix-1\nkey_file=/x" }, false},
		"fp format":           {func(c *OracleCredential) { c.Secret.Fingerprint = "abc" }, false},
		"fp mismatch":         {func(c *OracleCredential) { c.Secret.Fingerprint = smallFP }, false},
		"not pem":             {func(c *OracleCredential) { c.Secret.key = []byte("hello") }, false},
		"encrypted legacy":    {func(c *OracleCredential) { c.Secret.key = encPEM }, false},
		"encrypted pkcs8":     {func(c *OracleCredential) { c.Secret.key = enc8PEM }, false},
		"small key":           {func(c *OracleCredential) { c.Secret.key, c.Secret.Fingerprint = smallPEM, smallFP }, false},
		"empty key":           {func(c *OracleCredential) { c.Secret.key = nil }, false},
		"corrupt der": {func(c *OracleCredential) {
			c.Secret.key = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: []byte{9}})
		}, false},
		"wrong pem type": {func(c *OracleCredential) {
			c.Secret.key = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: []byte{9}})
		}, false},
		"oversize key": {func(c *OracleCredential) { c.Secret.key = bytes.Repeat([]byte("a"), 20000) }, false},
	}
	for name, tc := range cases {
		c := testCred(t)
		tc.mut(&c)
		err := ValidateOracle(c)
		if tc.ok != (err == nil) {
			t.Errorf("%s: err = %v, want ok=%v", name, err, tc.ok)
		}
		if err != nil {
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("%s: err %v is not ErrInvalid", name, err)
			}
			if strings.Contains(err.Error(), canaryLine(p)) {
				t.Errorf("%s: error leaks key material", name)
			}
		}
	}
}

func TestOracleMinBitsFromEnv(t *testing.T) {
	t.Setenv("APP_CLOUD_OCI_MIN_RSA_BITS", "4096")
	if err := ValidateOracle(testCred(t)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("2048 bit key with a 4096 floor: err = %v", err)
	}
}

type fakeSealer struct{ ns, key, plain string }

func (f *fakeSealer) SetValue(_ context.Context, ns, key, plain string) error {
	f.ns, f.key, f.plain = ns, key, plain
	return nil
}

func TestSealedKeyCanaryNeverLeaks(t *testing.T) {
	ctx := context.Background()
	c := testCred(t)
	canary := canaryLine(c.Secret.key)

	sealer := &fakeSealer{}
	ref, err := SealOracleSecret(ctx, sealer, "cloud-oci", "prod", c.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if ref != "cloud-oci/prod" || !strings.Contains(sealer.plain, canary) {
		t.Fatalf("ref %q or sealed bundle missing the key", ref)
	}

	dbPath := filepath.Join(t.TempDir(), "t.db")
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{Rows: db, Providers: DefaultRegistry()}
	in := Input{Provider: ProviderOracle, AuthMethod: AuthStatic, CredentialRef: ref,
		Config: Config{Region: c.Region, AccountID: c.TenancyOCID, Compartment: c.CompartmentOCID}}
	conn, err := s.Create(ctx, "u", "oci-prod", in)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	// API structs and every formatter.
	apiJSON, _ := json.Marshal(conn)
	secJSON, _ := json.Marshal(c.Secret)
	credJSON, _ := json.Marshal(c)
	for name, out := range map[string]string{
		"conn json": string(apiJSON), "secret json": string(secJSON), "cred json": string(credJSON),
		"%v": fmt.Sprintf("%v", c), "%+v": fmt.Sprintf("%+v", c), "%#v": fmt.Sprintf("%#v", c),
		"secret string": c.Secret.String(), "conn %+v": fmt.Sprintf("%+v", conn),
	} {
		if strings.Contains(out, canary) || strings.Contains(out, "PRIVATE KEY") {
			t.Errorf("%s leaks the key: %s", name, out)
		}
	}

	// The database files hold only the reference.
	entries, _ := os.ReadDir(filepath.Dir(dbPath))
	for _, e := range entries {
		raw, err := os.ReadFile(filepath.Join(filepath.Dir(dbPath), e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(raw, []byte(canary)) {
			t.Errorf("database file %s holds the key", e.Name())
		}
	}

	// A resolved bundle round trips without the key showing in the parsed form.
	back, err := ParseOracleSecret(secretsource.NewValue([]byte(sealer.plain), "1"))
	if err != nil || ValidateOracle(OracleCredential{TenancyOCID: tTenancy, CompartmentOCID: tCompartment, Region: "us-phoenix-1", Secret: back}) != nil {
		t.Fatalf("round trip: %v", err)
	}
	if _, err := ParseOracleSecret(secretsource.NewValue([]byte("{"+canary), "1")); err == nil || strings.Contains(err.Error(), canary) {
		t.Fatalf("bad bundle err = %v", err)
	}
}

func TestMaterializeOracleFilesAndCleanup(t *testing.T) {
	c := testCred(t)
	canary := canaryLine(c.Secret.key)
	base := t.TempDir()
	var dir string
	err := WithOracleCredential(base, "/run/lr-cred/credential", c, func(h *OracleCredFiles) error {
		dir = h.Dir()
		if strings.Contains(h.Dir()+h.ConfigPath(), canary) {
			t.Error("paths carry key material")
		}
		st, err := os.Stat(dir)
		if err != nil || st.Mode().Perm() != 0o700 {
			t.Errorf("dir mode = %v, %v", st.Mode().Perm(), err)
		}
		for _, f := range []string{OracleConfigFileName, OracleKeyFileName} {
			st, err := os.Stat(filepath.Join(dir, f))
			if err != nil || st.Mode().Perm() != 0o600 {
				t.Errorf("%s mode = %v, %v", f, st.Mode().Perm(), err)
			}
		}
		cfg, _ := os.ReadFile(h.ConfigPath())
		want := "[DEFAULT]\nuser=" + tUser + "\nfingerprint=" + c.Secret.Fingerprint + "\ntenancy=" + tTenancy +
			"\nregion=us-phoenix-1\nkey_file=/run/lr-cred/credential/" + OracleKeyFileName + "\n"
		if string(cfg) != want {
			t.Errorf("config =\n%s\nwant\n%s", cfg, want)
		}
		key, _ := os.ReadFile(filepath.Join(dir, OracleKeyFileName)) //nolint:gosec // test reads the dir it just materialized
		if !bytes.Equal(key, c.Secret.key) {
			t.Error("key file differs from the sealed key")
		}
		line := "provider said: " + canary + " and " + string(c.Secret.key)
		if got := h.Redact(line); strings.Contains(got, canary) || !strings.Contains(got, secretsource.Mask) {
			t.Errorf("redact left key material: %s", got)
		}
		return errors.New("step failed")
	})
	if err == nil || err.Error() != "step failed" {
		t.Fatalf("err = %v", err)
	}
	if _, serr := os.Stat(dir); !os.IsNotExist(serr) {
		t.Fatalf("credential dir survived: %v", serr)
	}
	if _, err := MaterializeOracle("relative", "", c); err == nil {
		t.Fatal("relative base accepted")
	}
	if _, err := MaterializeOracle(base, "/run/x\nkey_file=/etc/passwd", c); err == nil {
		t.Fatal("mount dir with newline accepted")
	}
}

// Signing string from the published OCI request signing example (docs.oracle.com, Content/API/Concepts/signingrequests.htm).
func TestOracleSigningStringMatchesPublishedExample(t *testing.T) {
	u := "https://iaas.us-phoenix-1.oraclecloud.com/20160918/instances?availabilityDomain=Pjwf%3A%20PHX-AD-1" +
		"&compartmentId=ocid1.compartment.oc1..aaaaaaaam3we6vgnherjq5q2idnccdflvjsnog7mlr6rtdb25gilchfeyjxa" +
		"&displayName=TeamXInstances&volumeId=ocid1.volume.oc1.phx.abyhqljrgvttnlx73nmrwfaux7kcvzfs3s66izvxf2h4lgvyndsdsnoiwr5q"
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("Date", "Thu, 05 Jan 2014 21:31:40 GMT")
	want := "date: Thu, 05 Jan 2014 21:31:40 GMT\n" +
		"(request-target): get /20160918/instances?availabilityDomain=Pjwf%3A%20PHX-AD-1" +
		"&compartmentId=ocid1.compartment.oc1..aaaaaaaam3we6vgnherjq5q2idnccdflvjsnog7mlr6rtdb25gilchfeyjxa" +
		"&displayName=TeamXInstances&volumeId=ocid1.volume.oc1.phx.abyhqljrgvttnlx73nmrwfaux7kcvzfs3s66izvxf2h4lgvyndsdsnoiwr5q\n" +
		"host: iaas.us-phoenix-1.oraclecloud.com"
	if got := oracleSigningString(req); got != want {
		t.Fatalf("signing string =\n%s\nwant\n%s", got, want)
	}
	post, _ := http.NewRequest(http.MethodPost, u, nil)
	k, _, _ := testKey(t)
	if err := signOracleRequest(post, "k", k, time.Time{}); err == nil {
		t.Fatal("non GET request was signed")
	}
}

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

var authRe = regexp.MustCompile(`^Signature version="1",keyId="([^"]+)",algorithm="rsa-sha256",headers="date \(request-target\) host",signature="([A-Za-z0-9+/=]+)"$`)

func TestOracleHealthCheckFakeTransport(t *testing.T) {
	c := testCred(t)
	canary := canaryLine(c.Secret.key)
	for _, tc := range []struct {
		status int
		err    error
		want   string
	}{
		{200, nil, ClassOK}, {401, nil, ClassAuthFailed}, {404, nil, ClassAuthFailed},
		{400, nil, ClassConfigInvalid}, {500, nil, ClassUnknown}, {0, errors.New("dial refused"), ClassUnreachable},
	} {
		var seen *http.Request
		chk := OracleHTTPChecker{
			Client: &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
				seen = r
				if tc.err != nil {
					return nil, tc.err
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(`{"code":"x"}`)), Header: http.Header{}}, nil
			})},
			BaseURL: func(region string) string { return "https://iaas." + region + ".example.test" },
		}
		p := OracleProvider{Checker: chk}
		bundle, _ := json.Marshal(oracleSealed{User: tUser, Fingerprint: c.Secret.Fingerprint, Key: string(c.Secret.key)})
		target := Target{
			Connection: Connection{Provider: ProviderOracle, AuthMethod: AuthSecretRef,
				Config: Config{Region: c.Region, AccountID: tTenancy, Compartment: tCompartment}},
			Credential: secretsource.NewValue(bundle, "1"),
		}
		class, err := p.Check(context.Background(), target)
		if class != tc.want {
			t.Errorf("status %d: class = %s (%v), want %s", tc.status, class, err, tc.want)
		}
		if err != nil && strings.Contains(err.Error(), canary) {
			t.Errorf("error leaks key: %v", err)
		}
		if seen == nil {
			t.Fatal("no request sent")
		}
		if seen.Method != http.MethodGet || seen.URL.Path != "/20160918/instances" || seen.URL.Query().Get("compartmentId") != tCompartment {
			t.Errorf("request = %s %s", seen.Method, seen.URL)
		}
		m := authRe.FindStringSubmatch(seen.Header.Get("Authorization"))
		if m == nil {
			t.Fatalf("authorization header shape: %q", seen.Header.Get("Authorization"))
		}
		if m[1] != tTenancy+"/"+tUser+"/"+c.Secret.Fingerprint {
			t.Errorf("keyId = %s", m[1])
		}
		sig, _ := base64.StdEncoding.DecodeString(m[2])
		digest := sha256.Sum256([]byte(oracleSigningString(seen)))
		if err := rsa.VerifyPKCS1v15(&tKey.PublicKey, crypto.SHA256, digest[:], sig); err != nil {
			t.Errorf("signature does not verify: %v", err)
		}
		for k, v := range seen.Header {
			if strings.Contains(strings.Join(v, ""), canary) {
				t.Errorf("header %s carries key material", k)
			}
		}
	}

	p := OracleProvider{Checker: OracleHTTPChecker{}}
	if class, _ := p.Check(context.Background(), Target{Connection: Connection{AuthMethod: AuthOIDCFederation}}); class != ClassUnsupported {
		t.Errorf("oidc class = %s", class)
	}
	if class, _ := p.Check(context.Background(), Target{Connection: Connection{AuthMethod: AuthSecretRef}}); class != ClassCredentialUnresolved {
		t.Errorf("empty credential class = %s", class)
	}
}

func TestPolicyStatementsLeastPrivilege(t *testing.T) {
	for _, comp := range []string{tCompartment, "levelrail"} {
		got, err := PolicyStatements(comp, "levelrail-iac")
		if err != nil || len(got) == 0 {
			t.Fatalf("%s: %v", comp, err)
		}
		forbidden := regexp.MustCompile(`(?i)\bin tenancy\b|all-resources|\busers\b|\bgroups\b|\bpolicies\b|dynamic-groups|\bcompartments\b|identity|domains|\bkeys\b|vaults|secret`)
		for _, s := range got {
			if !strings.HasPrefix(s, "Allow group levelrail-iac to manage ") || !strings.HasSuffix(s, comp) {
				t.Errorf("statement not compartment scoped: %s", s)
			}
			if forbidden.MatchString(s) {
				t.Errorf("statement grants too much: %s", s)
			}
		}
	}
	for _, bad := range [][2]string{{tTenancy, "g"}, {"tenancy", "g"}, {"root", "g"}, {"c", "g; Allow group x to manage all-resources in tenancy"}, {"c\nx", "g"}, {"", "g"}} {
		if _, err := PolicyStatements(bad[0], bad[1]); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}
