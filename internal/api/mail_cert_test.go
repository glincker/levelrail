package api

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/mailcert"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/volumefiles"
)

// Every PEM private key header shape, so a leak of any key form is caught.
var pemKeyMarkers = []string{"PRIVATE KEY", "BEGIN EC PRIVATE KEY", "BEGIN RSA PRIVATE KEY", "BEGIN ENCRYPTED PRIVATE KEY"}

func leaksKey(s string) bool {
	for _, m := range pemKeyMarkers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

type mailCertSecretsFake struct {
	sealed  map[string]string
	deleted []string
}

func (f *mailCertSecretsFake) SetValue(_ context.Context, svc, key, v string) error {
	f.sealed[svc+"/"+key] = v
	return nil
}
func (f *mailCertSecretsFake) DeleteAll(_ context.Context, svc string) error {
	f.deleted = append(f.deleted, svc)
	return nil
}

type mailCertNodeFake struct {
	*fakeExecAppRuntime
	files   map[string][]byte
	private map[string]bool
	// failKeyWritesAfter, when set, fails live key writes after that many succeed.
	failKeyWritesAfter int
	keyWrites          int
}

func (n *mailCertNodeFake) VolumeList(_ context.Context, _, dir string, _ int) ([]volumefiles.Entry, bool, error) {
	var out []volumefiles.Entry
	for p := range n.files {
		if strings.HasPrefix(p, dir+"/") {
			mode := "-rw-r--r--"
			if n.private[p] {
				mode = "-rw-------"
			}
			out = append(out, volumefiles.Entry{Path: p, Mode: mode})
		}
	}
	return out, false, nil
}
func (n *mailCertNodeFake) VolumeRead(_ context.Context, _, p string) (io.ReadCloser, error) {
	b, ok := n.files[p]
	if !ok {
		return nil, volumefiles.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}
func (n *mailCertNodeFake) VolumeWrite(_ context.Context, _, p string, r io.Reader, _ int64) error {
	b, _ := io.ReadAll(r)
	n.files[p], n.private[p] = b, false
	return nil
}
func (n *mailCertNodeFake) VolumeWritePrivate(_ context.Context, _, p string, r io.Reader, _ int64) error {
	b, _ := io.ReadAll(r)
	if p == mailcert.KeyPath && n.failKeyWritesAfter > 0 {
		n.keyWrites++
		if n.keyWrites > n.failKeyWritesAfter {
			return errors.New("injected write failure")
		}
	}
	n.files[p], n.private[p] = b, true
	return nil
}
func (n *mailCertNodeFake) VolumeRemove(_ context.Context, _, p string) error {
	delete(n.files, p)
	return nil
}

func mailCertTestPair(t *testing.T, host string) (string, string) {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(30 * 24 * time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &k.PublicKey, k)
	if err != nil {
		t.Fatal(err)
	}
	kd, _ := x509.MarshalPKCS8PrivateKey(k)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kd}))
}

func TestMailCertUploadNeverLeaksKeyAndRollsBack(t *testing.T) {
	t.Setenv("APP_MAIL_CERT_VERIFY_SECONDS", "1")
	t.Setenv("APP_MAIL_CERT_POLL_SECONDS", "1")
	secrets := &mailCertSecretsFake{sealed: map[string]string{}}
	rt, db := newTestRouterWithDomainTLSCertSecrets(t, secrets)
	node := &mailCertNodeFake{fakeExecAppRuntime: &fakeExecAppRuntime{}, files: map[string][]byte{}, private: map[string]bool{}}
	rt.execRuntime = func(string) (docker.Runtime, error) { return node, nil }
	var logs bytes.Buffer
	rt.logger = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	defer func() {
		if leaksKey(logs.String()) {
			t.Fatal("log output carries key material")
		}
	}()
	cookie := loginTestSession(t, rt, db)
	svc := store.DesiredService{Name: "mail", Image: "i:1", Port: mailWebPort, TemplateID: mailTemplateID,
		Env:     map[string]string{"MAIL_HOSTNAME": "mail.example.test"},
		Volumes: []store.ServiceVolume{{Name: "app-mail-mail_config", ContainerPath: "/mox/config"}}}
	if err := db.SaveDesiredService(context.Background(), svc); err != nil {
		t.Fatal(err)
	}
	certPEM, keyPEM := mailCertTestPair(t, "mail.example.test")
	_, otherKey := mailCertTestPair(t, "mail.example.test")
	body := func(cert, key string) string {
		b, _ := json.Marshal(map[string]any{"cert": cert, "key": key, "restart_now": true})
		return strings.TrimSuffix(string(b), "}") + mailStepUp + "}"
	}
	var bodies []string
	call := func(method, path, b string) int {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, b))
		bodies = append(bodies, rec.Body.String())
		return rec.Code
	}

	if code := call(http.MethodPut, "/api/v1/mail/cert", body(certPEM, otherKey)); code != http.StatusBadRequest {
		t.Fatalf("mismatched pair: %d", code)
	}
	if code := call(http.MethodPost, "/api/v1/mail/cert/obtain", `{}`); code != http.StatusNotImplemented {
		t.Fatalf("obtain: %d", code)
	}
	if code := call(http.MethodPut, "/api/v1/mail/cert", body(certPEM, keyPEM)); code != http.StatusBadGateway {
		t.Fatalf("upload without a serving engine should roll back, got %d: %s", code, bodies[len(bodies)-1])
	}
	if !strings.Contains(bodies[len(bodies)-1], mailcert.OutcomeRolledBack) {
		t.Fatalf("want rolled_back: %s", bodies[len(bodies)-1])
	}
	if secrets.sealed["mail-tls-cert/mail/"+mailCertKeyPEMEnvKey] == "" {
		t.Fatal("key was not sealed")
	}
	if _, left := node.files[mailcert.KeyPath]; left {
		t.Fatal("rollback with no previous pair must remove the managed key")
	}
	got, _ := db.GetDesiredService(context.Background(), "mail")
	if _, set := got.Env["MAIL_TLS_CERT_FILE"]; set {
		t.Fatal("rollback must restore the original certificate path")
	}

	call(http.MethodGet, "/api/v1/mail/cert", "")
	rows, err := db.ListAuditEntries(context.Background(), 500, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]bool{}
	for _, row := range rows {
		actions[row.Action] = true
		raw, _ := json.Marshal(row)
		if leaksKey(string(raw)) {
			t.Fatalf("audit row carries key material: %s", raw)
		}
	}
	if !actions["mail.cert.uploaded"] || !actions["mail.cert.rolled_back"] {
		t.Fatalf("audit actions = %v", actions)
	}
	for _, b := range bodies {
		if leaksKey(b) {
			t.Fatalf("response carries key material: %s", b)
		}
	}
}

func TestMailCertUploadLocksRecoversAndCleansUp(t *testing.T) {
	t.Setenv("APP_MAIL_CERT_VERIFY_SECONDS", "1")
	t.Setenv("APP_MAIL_CERT_POLL_SECONDS", "1")
	secrets := &mailCertSecretsFake{sealed: map[string]string{}}
	rt, db := newTestRouterWithDomainTLSCertSecrets(t, secrets)
	var logs bytes.Buffer
	rt.logger = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	node := &mailCertNodeFake{fakeExecAppRuntime: &fakeExecAppRuntime{}, files: map[string][]byte{}, private: map[string]bool{}}
	rt.execRuntime = func(string) (docker.Runtime, error) { return node, nil }
	cookie := loginTestSession(t, rt, db)
	svc := store.DesiredService{Name: "mail", Image: "i:1", Port: mailWebPort, TemplateID: mailTemplateID,
		Env:     map[string]string{"MAIL_HOSTNAME": "mail.example.test"},
		Volumes: []store.ServiceVolume{{Name: "app-mail-mail_config", ContainerPath: "/mox/config"}}}
	if err := db.SaveDesiredService(context.Background(), svc); err != nil {
		t.Fatal(err)
	}
	certPEM, keyPEM := mailCertTestPair(t, "mail.example.test")
	b, _ := json.Marshal(map[string]any{"cert": certPEM, "key": keyPEM, "restart_now": true})
	body := strings.TrimSuffix(string(b), "}") + mailStepUp + "}"
	put := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/mail/cert", body))
		if leaksKey(rec.Body.String()) {
			t.Fatalf("response carries key material: %s", rec.Body.String())
		}
		return rec
	}

	unlock, ok := rt.lockMailCert("mail")
	if !ok {
		t.Fatal("lock")
	}
	if rec := put(); rec.Code != http.StatusConflict {
		t.Fatalf("concurrent install = %d, want 409", rec.Code)
	}
	unlock()

	fresh, _ := json.Marshal(mailcert.Journal{Phase: mailcert.PhaseSwapping, StartedAt: time.Now(), HadPrev: false})
	node.files[mailcert.JournalPath] = fresh
	if rec := put(); rec.Code != http.StatusConflict {
		t.Fatalf("install inside another journal's lease = %d, want 409", rec.Code)
	}

	// A stale journal from a crash mid swap: recovery removes the half written pair first.
	stale, _ := json.Marshal(mailcert.Journal{Phase: mailcert.PhaseSwapping, StartedAt: time.Now().Add(-time.Hour), HadPrev: false})
	node.files[mailcert.JournalPath] = stale
	node.files[mailcert.KeyPath] = []byte(keyPEM)
	rec := put()
	if !strings.Contains(rec.Body.String(), `"recovered":"rolled_back"`) {
		t.Fatalf("stale journal not recovered: %d %s", rec.Code, rec.Body.String())
	}

	// A restore that cannot write is reported as rollback_failed, not as a plain failure.
	node.files[mailcert.CertPath], node.files[mailcert.KeyPath] = []byte(certPEM), []byte(keyPEM)
	node.failKeyWritesAfter = 1
	rec = put()
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), mailcert.OutcomeRollbackFailed) || !strings.Contains(rec.Body.String(), `"error":"rollback_failed`) {
		t.Fatalf("rollback failure = %d %s", rec.Code, rec.Body.String())
	}
	rows, _ := db.ListAuditEntries(context.Background(), 500, nil, store.AuditEntryFilter{})
	actions := map[string]bool{}
	for _, row := range rows {
		actions[row.Action] = true
	}
	if !actions["mail.cert.recovered"] || !actions["mail.cert.rollback_failed"] {
		t.Fatalf("audit actions = %v", actions)
	}

	if _, err := rt.deleteApp(context.Background(), "mail"); err != nil {
		t.Fatal(err)
	}
	if len(secrets.deleted) != 1 || secrets.deleted[0] != mailCertSecretsKey("mail") {
		t.Fatalf("sealed pair not dropped on delete: %v", secrets.deleted)
	}
	if leaksKey(logs.String()) {
		t.Fatal("log output carries key material")
	}
}
