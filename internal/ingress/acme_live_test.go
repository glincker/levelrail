package ingress

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	smallstepacme "github.com/smallstep/certificates/acme"

	"github.com/GLINCKER/levelrail/internal/dockertest"
)

// acmeServerHandler is the minimal JSON shape for Caddy's own embedded
// ACME server module (http.handlers.acme_server, verified against
// modules/caddypki/acmeserver/acmeserver.go's Handler struct in the
// vendored caddyserver/caddy/v2 source this module pins): a real,
// local, RFC 8555-speaking CA backed by the pki app's own CA. This is
// test-only scaffolding, never exercised by production code (the
// production ingress never runs its own ACME server), so it lives here
// rather than among config.go's structs.
type acmeServerHandler struct {
	Handler    string   `json:"handler"`
	Challenges []string `json:"challenges,omitempty"`
}

// acmeIssuerWithTrust adds Caddy's real "trusted_roots_pem_files" field
// (verified against modules/caddytls/acmeissuer.go's ACMEIssuer struct
// in the vendored source, same file NewACMEIssuer's own doc comment
// verifies its three fields against) on top of an ACMEIssuer built by
// the real, production NewACMEIssuer constructor. Test-only: an
// operator pointed at a real ACME CA (Let's Encrypt or otherwise
// publicly trusted) never needs this, only this test's own
// self-signed, sandboxed ACME endpoint does, which is why it lives here
// rather than as a new field on config.go's own ACMEIssuer struct. Go's
// encoding/json flattens an embedded struct's own tagged fields into
// the same JSON object, so this still marshals to the exact real Caddy
// wire shape (module/email/ca plus trusted_roots_pem_files, all at the
// top level).
type acmeIssuerWithTrust struct {
	ACMEIssuer
	TrustedRootsPEMFiles []string `json:"trusted_roots_pem_files,omitempty"`
}

// mintTestCA generates a throwaway, in-memory-only self-signed CA and a
// leaf certificate for host, signed by it. This stands in for a real
// certificate authority solely to give the "acmeca" test server (see
// TestACMEIssuer_RealIssuanceAgainstLocalACMEServer) its own HTTPS
// identity: Caddy's ACME server module hardcodes "https" in the
// self-referential URLs it returns from its own directory endpoint
// (smallstep/certificates/acme/api/middleware.go's reqURL), so serving
// it over plain HTTP breaks its own directory response regardless of
// the scheme the request actually arrived on; a real, if sandboxed, TLS
// identity is not optional here. This CA is unrelated to the "local" CA
// the pki app (newInternalPKIApp) manages, which is what the acme_server
// handler itself uses to sign the certificates it issues to ACME
// clients: two different roots for two different jobs, transport
// security for the ACME endpoint itself versus the certificates that
// endpoint hands out.
func mintTestCA(t *testing.T, host string) (rootCertPEM, leafCertPEM, leafKeyPEM string) {
	t.Helper()

	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating root key: %v", err)
	}
	rootTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "levelrail test root CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("creating root cert: %v", err)
	}
	rootCert, err := x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatalf("parsing root cert: %v", err)
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating leaf key: %v", err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: host},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if ip := net.ParseIP(host); ip != nil {
		leafTemplate.IPAddresses = []net.IP{ip}
	} else {
		leafTemplate.DNSNames = []string{host}
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, rootCert, &leafKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("creating leaf cert: %v", err)
	}

	leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		t.Fatalf("marshaling leaf key: %v", err)
	}

	pemEncode := func(blockType string, der []byte) string {
		return string(pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}))
	}
	return pemEncode("CERTIFICATE", rootDER), pemEncode("CERTIFICATE", leafDER), pemEncode("EC PRIVATE KEY", leafKeyDER)
}

// TestACMEIssuer_RealIssuanceAgainstLocalACMEServer is the end-to-end
// proof ADR 005 and docs/roadmap.md's "Real public ACME" item named as
// the remaining gap: real certificate issuance over the real ACME
// protocol (RFC 8555), exercised through this package's actual
// production code path (BuildRoutesConfig with ACMEEnabled, which
// builds NewACMEIssuer's exact output and feeds it to the same
// Driver.Apply/caddy.Load call production uses) and verified against a
// real ACME implementation, not a mock.
//
// No internet reachability or public domain exists in this sandbox, so
// "a real ACME implementation" here is Caddy's own embedded ACME server
// module (http.handlers.acme_server, caddypki/acmeserver), already
// pulled into this binary by driver.go's blank import of
// github.com/caddyserver/caddy/v2/modules/standard: no new dependency.
// letsencrypt/pebble, the task's own first suggestion, was checked
// first (`grep pebble go.sum`) and turned out to be a phantom entry:
// present in go.sum for module-graph completeness only, imported by
// nothing caddy or certmagic actually build (confirmed by grepping
// both modules' source trees), so adding it fresh would have meant a
// brand new dependency and its own transitive graph for strictly less
// coverage than the ACME server module already compiled into this
// binary.
//
// The test builds one Caddy config with three servers sharing one
// process: "acmeca" (a real, if sandboxed, HTTPS listener hosting
// acme_server, backed by a fresh local CA), "challenge" (a plain HTTP
// listener that exists purely to answer the HTTP-01 challenge request,
// see its own comment below), and "app" (BuildRoutesConfig's real
// production output, TLS automation pointed at the acmeca server's own
// directory URL). smallstep's ACME authority, acmeserver's underlying
// implementation, validates the HTTP-01 challenge by connecting to the
// identifier host on port 80 unless overridden; InsecurePortHTTP01 is
// smallstep's own test-only escape hatch for exactly this (Caddy's own
// caddytest/integration/acme_test.go sets the same package variable the
// same way). "localhost" is used as the certificate subject because it
// resolves to loopback everywhere with no DNS setup required, the same
// identifier Caddy's own ACME integration tests use for this reason.
//
// Not safe under `go test -count=N` (N>1) in one process: Caddy's
// in-process certificate cache is a package-level singleton that
// outlives any single caddy.Load call, by design (a real reconciler
// re-applying the same config shouldn't re-request a certificate it
// already holds). A second invocation in the same process finds
// "localhost" already cached from the first run's now-discarded local
// CA and serves that stale certificate straight from cache instead of
// obtaining a fresh one, which then fails to verify against the new
// run's own root. A normal `go test` invocation, which is how this
// repo's CI always runs (one process per package, see nightly.yml), is
// unaffected; this was confirmed by running the whole package suite
// standalone, repeatedly, each in its own process.
func TestACMEIssuer_RealIssuanceAgainstLocalACMEServer(t *testing.T) {
	dockertest.SkipIfShort(t)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := fmt.Fprint(w, "hello over real ACME"); err != nil {
			t.Errorf("backend: writing response: %v", err)
		}
	}))
	defer backend.Close()
	backendAddr := backend.Listener.Addr().String()

	acmeHost := "127.0.0.1"
	acmeAddr := fmt.Sprintf("%s:%d", acmeHost, freePort(t))
	appAddr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	caURL := fmt.Sprintf("https://%s/acme/local/directory", acmeAddr)
	storageDir := t.TempDir()

	// challengeAddr is where Caddy's own HTTP-01 solver for the "app"
	// server's automation policy answers challenge requests: a plain,
	// unencrypted HTTP listener, since http01Validate (below) always
	// builds a "http://" challenge URL. It is deliberately a separate
	// server from "acmeca": "app" itself can't take the request directly
	// (its only listener speaks TLS, and a plain HTTP-01 GET isn't a
	// valid TLS ClientHello), but any caddyhttp server answers a
	// challenge it recognizes regardless of host or route
	// (modules/caddyhttp/server.go's ServeHTTP calls
	// tlsApp.HandleHTTPChallenge before normal routing), so an otherwise
	// empty server here is enough.
	challengeAddr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	_, challengePortStr, err := net.SplitHostPort(challengeAddr)
	if err != nil {
		t.Fatalf("splitting challenge addr: %v", err)
	}
	var challengePort int
	if _, err := fmt.Sscanf(challengePortStr, "%d", &challengePort); err != nil {
		t.Fatalf("parsing challenge port: %v", err)
	}

	// smallstep's authority (the CA backing our local acme_server)
	// otherwise validates HTTP-01 on real port 80, which this sandbox
	// can't bind without root; this is its own documented test-only
	// override for exactly that, pointed at challengeAddr above, not
	// the acme_server's own port (a real CA and the port it validates
	// challenges on are unrelated; the two coinciding here would have
	// been a coincidence, not something to rely on). Reset afterward
	// since it is a package-level var shared by every test in this
	// binary.
	smallstepacme.InsecurePortHTTP01 = challengePort
	defer func() { smallstepacme.InsecurePortHTTP01 = 0 }()

	// The acme_server handler always returns "https://" self-links from
	// its own directory endpoint regardless of the scheme it was
	// actually reached over (smallstep/certificates/acme/api/
	// middleware.go hardcodes it), so the acmeca server below needs a
	// real TLS identity, not just a plain HTTP listener.
	transportRootPEM, transportLeafPEM, transportLeafKeyPEM := mintTestCA(t, acmeHost)
	transportRootPath := filepath.Join(t.TempDir(), "acmeca-root.pem")
	if err := os.WriteFile(transportRootPath, []byte(transportRootPEM), 0o600); err != nil {
		t.Fatalf("writing acmeca root CA to disk: %v", err)
	}

	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "app",
		ListenAddr: appAddr,
		Routes: []ProxyRoute{
			{Hosts: []string{"localhost"}, BackendDial: backendAddr},
		},
		TLS:              true,
		ACMEEnabled:      true,
		ACMEEmail:        "ops@example.com",
		ACMEDirectoryURL: caURL,
		StorageDir:       storageDir,
	})
	if err != nil {
		t.Fatalf("BuildRoutesConfig() error: %v", err)
	}

	// Swap in the trust-augmented issuer (see acmeIssuerWithTrust's doc
	// comment): same Module/Email/CA values NewACMEIssuer produced,
	// carrying a reference to this test's own sandboxed root so the
	// ACME client can complete the TLS handshake to reach it.
	policy := &cfg.Apps.TLS.Automation.Policies[0]
	issuer, ok := policy.Issuers[0].(ACMEIssuer)
	if !ok {
		t.Fatalf("policy.Issuers[0] = %T, want ACMEIssuer", policy.Issuers[0])
	}
	policy.Issuers[0] = acmeIssuerWithTrust{ACMEIssuer: issuer, TrustedRootsPEMFiles: []string{transportRootPath}}

	// BuildRoutesConfig's ACME branch deliberately leaves Apps.PKI nil
	// (real ACME has no local CA of its own to manage trust for, see
	// its own doc comment); this test's own local ACME server needs
	// one, added here with the same helper the internal-issuer branch
	// already uses. This is a second, distinct CA from the one just
	// minted above: that one secures the acmeca server's own transport,
	// this one is what acme_server actually signs issued certificates
	// with.
	cfg.Apps.PKI = newInternalPKIApp()
	cfg.Apps.TLS.Certificates = CertificatesConfig{
		"load_pem": []CertKeyPEMPair{{CertificatePEM: transportLeafPEM, KeyPEM: transportLeafKeyPEM}},
	}
	cfg.Apps.HTTP.Servers["acmeca"] = &Server{
		Listen: []string{acmeAddr},
		Routes: []Route{{
			Match:  []Matcher{{Host: []string{acmeHost}}},
			Handle: []any{acmeServerHandler{Handler: "acme_server", Challenges: []string{"http-01"}}},
		}},
		// The loaded transport cert above covers this host, so
		// automatic HTTPS skips issuing another one for it (see
		// CertKeyPEMPair's own doc comment) and just enables TLS on
		// this listener. DisableRedir avoids Caddy also trying to bind
		// the default HTTP port (80) for a redirect, which needs root.
		AutomaticHTTPS: &AutoHTTPSConfig{DisableRedir: true},
	}
	cfg.Apps.HTTP.Servers["challenge"] = &Server{
		Listen: []string{challengeAddr},
		Routes: []Route{{
			Handle: []any{StaticResponseHandler{Handler: "static_response", StatusCode: http.StatusNotFound}},
		}},
		// Plain HTTP only, on purpose: see challengeAddr's own comment
		// above. Never a TLS automation subject.
		AutomaticHTTPS: &AutoHTTPSConfig{Disabled: true},
	}

	d := New(testLogger(t))
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if err := d.Apply(ctx, cfg); err != nil {
		t.Fatalf("Driver.Apply() error: %v (real ACME issuance against the local ACME server did not complete)", err)
	}
	defer func() {
		if err := d.Stop(ctx); err != nil {
			t.Errorf("Driver.Stop() error: %v", err)
		}
	}()

	// Unlike the internal issuer (self-signed, fully synchronous),
	// obtaining a real ACME certificate is a real network exchange that
	// runs as a background job Apply() does not wait on; the retry loop
	// below is this test's actual proof, not just tolerance for
	// ordinary listener startup. A real TLS client, trusting nothing but
	// this test's own local CA root, completing a real handshake and
	// reading back the right SAN is what proves issuance succeeded.
	rootPEM, err := os.ReadFile(filepath.Join(storageDir, "pki", "authorities", "local", "root.crt")) //nolint:gosec // storageDir is this test's own t.TempDir() fixture path
	if err != nil {
		t.Fatalf("reading local CA root cert: %v", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(rootPEM) {
		t.Fatalf("no certificates parsed from local CA root cert file")
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				var dialer net.Dialer
				return dialer.DialContext(ctx, network, appAddr)
			},
			TLSClientConfig: &tls.Config{
				RootCAs:    pool,
				ServerName: "localhost",
			},
		},
	}

	body := getBodyWithRetryClient(t, client, "https://localhost/")
	const want = "hello over real ACME"
	if body != want {
		t.Errorf("response through Caddy over real ACME-issued TLS = %q, want %q", body, want)
	}

	rawConn, err := (&net.Dialer{Timeout: 5 * time.Second}).Dial("tcp", appAddr)
	if err != nil {
		t.Fatalf("dialing Caddy directly: %v", err)
	}
	tlsConn := tls.Client(rawConn, &tls.Config{RootCAs: pool, ServerName: "localhost"})
	defer func() {
		if err := tlsConn.Close(); err != nil {
			t.Logf("closing inspection connection: %v", err)
		}
	}()
	if err := tlsConn.Handshake(); err != nil {
		t.Fatalf("TLS handshake against the real ACME-issued certificate: %v", err)
	}
	leaf := tlsConn.ConnectionState().PeerCertificates[0]
	if err := leaf.VerifyHostname("localhost"); err != nil {
		t.Errorf("issued certificate does not verify for localhost: %v", err)
	}
	if leaf.NotAfter.Before(time.Now()) {
		t.Errorf("issued certificate already expired: notAfter=%s", leaf.NotAfter)
	}
	t.Logf("real ACME-issued certificate: subject=%s issuer=%s san=%v notAfter=%s",
		leaf.Subject, leaf.Issuer, leaf.DNSNames, leaf.NotAfter)
}
