// TestDomainTLSCert_Live_ServesUploadedCertificateInsteadOfACME is this
// feature's own whole-chain proof, the same shape
// TestMaintenanceMode_Live_TogglesResponseWithoutStoppingContainer
// establishes: a real build, a real running container, a real ingress
// reconcile pass, and a real HTTPS request through Caddy. What it
// specifically proves that no other live test does: a domain with a BYO
// certificate configured is served that exact certificate over TLS,
// instead of one from Caddy's own automatic internal issuer, on the
// very next ingress reconcile pass.
package e2e

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
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	ingressreconcile "github.com/GLINCKER/levelrail/internal/reconcile/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

// staticDomainTLSCertResolver is a hand-written fake for
// ingressreconcile.DomainTLSCertPEMResolver: this test exercises the
// reconciler-to-Caddy path with a real certificate, not
// internal/secrets' own encryption round trip (internal/api's
// TestHandleSetDomainTLSCert_RealSecretsManager_RoundTripsThroughEncryption
// already covers that), so a plain in-memory lookup is enough here.
type staticDomainTLSCertResolver map[string]string

func (r staticDomainTLSCertResolver) Resolve(_ context.Context, serviceName, envKey string) (string, error) {
	return r[serviceName+"/"+envKey], nil
}

// genSelfSignedCertKeyPEM generates a real, self-signed leaf
// certificate/key pair for domain, PEM-encoding both, so this test can
// assert the live TLS connection's peer certificate is this exact
// certificate rather than one Caddy's own internal issuer minted.
func genSelfSignedCertKeyPEM(t *testing.T, domain string) (certPEM, keyPEM string, leaf *x509.Certificate) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: domain},
		DNSNames:     []string{domain},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		Issuer:       pkix.Name{CommonName: "Test BYO CA"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	leaf, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse generated certificate: %v", err)
	}
	certBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	keyBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return string(certBytes), string(keyBytes), leaf
}

func TestDomainTLSCert_Live_ServesUploadedCertificateInsteadOfACME(t *testing.T) {
	env := newLiveBuildEnv(t)

	const (
		serviceName = "levelrail-test-e2e-tls-cert"
		domain      = "e2e-tls-cert.levelrail.internal"
	)
	repo := "levelrail/test-e2e-tls-cert"
	tag := repo + ":e2etlscert1"

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = env.DockerCli.ImageRemove(cleanupCtx, tag, image.RemoveOptions{Force: true})
	})
	cleanupContainers(context.Background(), t, env.Runtime, serviceName)
	t.Cleanup(func() { cleanupContainers(context.Background(), t, env.Runtime, serviceName) })

	buildCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	res, err := env.BuildClient.Build(buildCtx, build.Request{ContextDir: "../fixtures/hello-e2e", Tag: tag}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	svcStore := openLiveStore(t)
	desired := store.DesiredService{
		Name:    serviceName,
		Image:   res.Tag,
		Port:    8080,
		Domains: []string{domain},
		Health:  &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/"}},
	}
	if err := svcStore.SaveDesiredService(buildCtx, desired); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	appCtrl := application.New(serviceName, svcStore, env.Runtime)
	appResult, err := appCtrl.Reconcile(buildCtx)
	if err != nil {
		t.Fatalf("application Controller.Reconcile() error = %v, result = %+v", err, appResult)
	}
	if len(appResult.Conditions) == 0 || appResult.Conditions[0].Status != "True" {
		t.Fatalf("application Controller.Reconcile() result = %+v, want a True Ready condition", appResult)
	}

	certPEM, keyPEM, wantLeaf := genSelfSignedCertKeyPEM(t, domain)
	uploadedAt := time.Now().UTC()
	if err := svcStore.SetDomainTLSCert(context.Background(), domain, uploadedAt, wantLeaf.NotAfter); err != nil {
		t.Fatalf("SetDomainTLSCert() error = %v", err)
	}
	key := store.DomainTLSCertSecretsKey(domain)
	resolver := staticDomainTLSCertResolver{
		key + "/" + store.DomainTLSCertCertificateEnvKey: certPEM,
		key + "/" + store.DomainTLSCertPrivateKeyEnvKey:  keyPEM,
	}

	caddyPort := freePort(t)
	caddyAddr := fmt.Sprintf("127.0.0.1:%d", caddyPort)
	adminAddr := fmt.Sprintf("127.0.0.1:%d", freePort(t))

	driver := ingress.New(nil)
	t.Cleanup(func() {
		if err := driver.Stop(context.Background()); err != nil {
			t.Errorf("Driver.Stop() error = %v", err)
		}
	})
	ingressCtrl := ingressreconcile.New(svcStore, env.Runtime, driver,
		ingressreconcile.WithServerName("e2e-tls-cert"),
		ingressreconcile.WithListenAddr(caddyAddr),
		ingressreconcile.WithAdminListen(adminAddr),
		ingressreconcile.WithStorageDir(t.TempDir()),
		ingressreconcile.WithDomainTLSCertSecrets(resolver),
	)

	ingressCtx, ingressCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer ingressCancel()
	if _, err := ingressCtrl.Reconcile(ingressCtx); err != nil {
		t.Fatalf("ingress Controller.Reconcile() error = %v", err)
	}

	gotLeaf := dialAndGetPeerLeafWithRetry(t, caddyAddr, domain)
	if !gotLeaf.Equal(wantLeaf) {
		t.Fatalf("served certificate serial = %v issuer = %v, want the uploaded certificate's serial = %v issuer = %v",
			gotLeaf.SerialNumber, gotLeaf.Issuer, wantLeaf.SerialNumber, wantLeaf.Issuer)
	}
}

// dialAndGetPeerLeafWithRetry opens a real TLS connection to addr with
// SNI set to serverName and returns the leaf certificate the server
// presented, retrying until Caddy's asynchronous config apply has
// actually taken effect, the same "settle, don't assume synchronous"
// shape getBodyWithRetry already uses for a plain HTTP request.
func dialAndGetPeerLeafWithRetry(t *testing.T, addr, serverName string) *x509.Certificate {
	t.Helper()

	deadline := time.Now().Add(8 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		dialer := &net.Dialer{Timeout: 2 * time.Second}
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{
			ServerName:         serverName,
			InsecureSkipVerify: true, //nolint:gosec // deliberate: this test's own self-signed cert is not in any trust store, see deploy_test.go's identical comment
		})
		if err != nil {
			lastErr = err
			time.Sleep(100 * time.Millisecond)
			continue
		}
		state := conn.ConnectionState()
		_ = conn.Close()
		if len(state.PeerCertificates) == 0 {
			lastErr = fmt.Errorf("no peer certificates presented")
			time.Sleep(100 * time.Millisecond)
			continue
		}
		return state.PeerCertificates[0]
	}

	t.Fatalf("TLS dial to %s (SNI %s) never succeeded within the retry window, last error: %v", addr, serverName, lastErr)
	return nil
}
