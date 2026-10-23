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
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/certflow"
	"github.com/GLINCKER/levelrail/internal/domaindoctor"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

const certFlowServerIP = "203.0.113.7"

type caaDNS struct {
	doctorFakeDNS
	recs []domaindoctor.CAARecord
}

func (c caaDNS) CAA(context.Context, string) ([]domaindoctor.CAARecord, error) { return c.recs, nil }

func seedCertIssuer(t *testing.T, db *store.DB, issuer string, notBefore, notAfter time.Time) {
	t.Helper()
	seedCertAt(t, db, "acme", issuer, notBefore, notAfter)
}

// seedCertAt stores a leaf under the certmagic issuer key segment keySeg.
func seedCertAt(t *testing.T, db *store.DB, keySeg, issuer string, notBefore, notAfter time.Time) {
	t.Helper()
	const domain = "shop.example.dev"
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: domain}, DNSNames: []string{domain},
		NotBefore: notBefore, NotAfter: notAfter,
	}
	parent := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: issuer}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := db.SaveCertStorageValue(context.Background(), "certificates/"+keySeg+"/"+domain+"/"+domain+".crt", pemBytes); err != nil {
		t.Fatal(err)
	}
}

func certFlowRouter(t *testing.T, dns domaindoctor.DNSProbe) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	rt, db := newTestRouter(t)
	rt.publicHost = certFlowServerIP
	rt.detectPublicIPs = func(context.Context) []string { return []string{certFlowServerIP} }
	seedRunningApp(t, db, "shop.example.dev")
	called := false
	rt.traffic.doctorProbes = &domaindoctor.Probes{DNS: dns, TLS: doctorFakeTLS{called: &called}, HTTP: doctorFakeTLS{called: &called}}
	t.Cleanup(func() { ingress.DefaultACMEFailures().ClearAll() })
	return rt, db, loginTestSession(t, rt, db)
}

func certFlowDo(t *testing.T, rt *Router, cookie *http.Cookie, method, path string, body any, out any) int {
	t.Helper()
	var raw string
	if body != nil {
		b, _ := json.Marshal(body)
		raw = string(b)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, raw))
	if out != nil && rec.Body.Len() > 0 {
		if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(out); err != nil {
			t.Fatalf("decode %s: %v (%s)", path, err, rec.Body.String())
		}
	}
	return rec.Code
}

func TestCertFlowPreflightOutcomes(t *testing.T) {
	caaBlock := []domaindoctor.CAARecord{{Name: "shop.example.dev", Tag: "issue", Value: "other-ca.example"}}
	tests := []struct {
		name      string
		dns       domaindoctor.DNSProbe
		query     string
		wantReady bool
		code      string
		state     certflow.ItemState
	}{
		{"points here", doctorFakeDNS{ips: []string{certFlowServerIP}}, "?email=me@shop.dev", true, certflow.CodeDNS, certflow.StatePass},
		{"points elsewhere", doctorFakeDNS{ips: []string{"198.51.100.4"}}, "?email=me@shop.dev", false, certflow.CodeDNS, certflow.StateFail},
		{"caa blocks", caaDNS{doctorFakeDNS{ips: []string{certFlowServerIP}}, caaBlock}, "?email=me@shop.dev", false, certflow.CodeCAA, certflow.StateFail},
		{"bad email", doctorFakeDNS{ips: []string{certFlowServerIP}}, "?email=nope", false, certflow.CodeEmail, certflow.StateFail},
		{"staging warns", doctorFakeDNS{ips: []string{certFlowServerIP}}, "?email=me@shop.dev&staging=true", true, certflow.CodeStaging, certflow.StateWarn},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt, _, cookie := certFlowRouter(t, tc.dns)
			var res certflow.Result
			if code := certFlowDo(t, rt, cookie, http.MethodGet, "/api/v1/domains/shop.example.dev/cert/preflight"+tc.query, nil, &res); code != http.StatusOK {
				t.Fatalf("status = %d", code)
			}
			if res.Ready != tc.wantReady {
				t.Fatalf("ready = %v, items = %+v", res.Ready, res.Items)
			}
			found := false
			for _, it := range res.Items {
				if it.Code == tc.code {
					found = true
					if it.State != tc.state {
						t.Fatalf("%s = %q, want %q (%s)", tc.code, it.State, tc.state, it.Detail)
					}
				}
			}
			if !found {
				t.Fatalf("no %s item in %+v", tc.code, res.Items)
			}
		})
	}
	rt, _, cookie := certFlowRouter(t, doctorFakeDNS{})
	if code := certFlowDo(t, rt, cookie, http.MethodGet, "/api/v1/domains/nobody.example.dev/cert/preflight", nil, nil); code != http.StatusNotFound {
		t.Fatalf("unknown domain = %d, want 404", code)
	}
}

func TestCertFlowPreflightRateLimitNextSafeTime(t *testing.T) {
	rt, _, cookie := certFlowRouter(t, doctorFakeDNS{ips: []string{certFlowServerIP}})
	now := time.Now()
	for i := 1; i <= 5; i++ {
		rt.certLedger.RecordFailure("shop.example.dev", now.Add(-time.Duration(i)*8*time.Minute), now)
	}
	var res certflow.Result
	certFlowDo(t, rt, cookie, http.MethodGet, "/api/v1/domains/shop.example.dev/cert/preflight?email=me@shop.dev", nil, &res)
	if res.Ready || res.NextSafeAt == nil {
		t.Fatalf("ready = %v next = %v", res.Ready, res.NextSafeAt)
	}
	want := now.Add(-40 * time.Minute).Add(time.Hour)
	if d := res.NextSafeAt.Sub(want); d < -2*time.Second || d > 2*time.Second {
		t.Fatalf("next safe = %v, want about %v", res.NextSafeAt, want)
	}
	var staged certflow.Result
	certFlowDo(t, rt, cookie, http.MethodGet, "/api/v1/domains/shop.example.dev/cert/preflight?email=me@shop.dev&staging=true", nil, &staged)
	if !staged.Ready {
		t.Fatalf("staging must bypass the production rate limit: %+v", staged.Items)
	}
}

func TestCertFlowGetBlocksThenApplies(t *testing.T) {
	rt, db, cookie := certFlowRouter(t, doctorFakeDNS{ips: []string{"198.51.100.4"}})
	var blocked getCertBlocked
	if code := certFlowDo(t, rt, cookie, http.MethodPost, "/api/v1/domains/shop.example.dev/cert", map[string]any{"email": "me@shop.dev"}, &blocked); code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", code)
	}
	if blocked.Preflight.Ready || blocked.Error == "" {
		t.Fatalf("blocked = %+v", blocked)
	}
	if s, _ := db.GetIngressSettings(context.Background()); s.ACMEEnabled {
		t.Fatal("a blocked preflight must not change settings")
	}

	rt2, db2, cookie2 := certFlowRouter(t, doctorFakeDNS{ips: []string{certFlowServerIP}})
	var st domainCertStatusResource
	if code := certFlowDo(t, rt2, cookie2, http.MethodPost, "/api/v1/domains/shop.example.dev/cert", map[string]any{"email": "me@shop.dev", "staging": true}, &st); code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", code)
	}
	s, _ := db2.GetIngressSettings(context.Background())
	if !s.ACMEEnabled || s.ACMEEmail != "me@shop.dev" || s.ACMEDirectoryURL != ACMEStagingDirectoryURL {
		t.Fatalf("settings = %+v", s)
	}
	if st.State != certFlowPending || !st.Staging || !st.AppliesToAll {
		t.Fatalf("status = %+v", st)
	}
	if rt2.certLedger.StartedAt("shop.example.dev").IsZero() {
		t.Fatal("start time not recorded")
	}
}

func TestCertFlowAcknowledgePortsOnly(t *testing.T) {
	res := certflow.Result{Items: []certflow.Item{
		{Code: certflow.CodePorts, State: certflow.StateFail, Detail: "closed"},
		{Code: certflow.CodeCAA, State: certflow.StateFail, Detail: "blocked"},
	}}
	if b := firstBlocker(res, []string{acknowledgePorts}); b == nil || b.Code != certflow.CodeCAA {
		t.Fatalf("blocker = %+v, want the CAA item", b)
	}
	if b := firstBlocker(res, nil); b == nil || b.Code != certflow.CodePorts {
		t.Fatalf("blocker = %+v, want ports", b)
	}
	if firstBlocker(certflow.Result{Items: []certflow.Item{{Code: certflow.CodePorts, State: certflow.StateFail}}}, []string{acknowledgePorts}) != nil {
		t.Fatal("acknowledged ports must not block")
	}
}

func TestCertFlowStatusProgress(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name    string
		setup   func(rt *Router, db *store.DB)
		state   string
		kind    certflow.Kind
		explain string
	}{
		{"off", func(*Router, *store.DB) {}, certFlowOff, certflow.KindNone, ""},
		{"pending", func(rt *Router, db *store.DB) {
			enableACMEForTest(t, db, "")
			rt.certLedger.MarkStarted("shop.example.dev", now)
		}, certFlowPending, certflow.KindNone, ""},
		{"failed with plain reason", func(_ *Router, db *store.DB) {
			enableACMEForTest(t, db, "")
			ingress.DefaultACMEFailures().Record(ingress.ACMEFailure{Identifier: "shop.example.dev", Error: "Timeout during connect (likely firewall problem)", At: now})
		}, certFlowFailed, certflow.KindNone, "could not reach this server on port 80"},
		{"issued", func(_ *Router, db *store.DB) {
			enableACMEForTest(t, db, "")
			seedCertIssuer(t, db, "R11", now.Add(-10*24*time.Hour), now.Add(80*24*time.Hour))
		}, certFlowIssued, certflow.KindLetsEncrypt, ""},
		{"issued staging", func(_ *Router, db *store.DB) {
			enableACMEForTest(t, db, ACMEStagingDirectoryURL)
			seedCertIssuer(t, db, "(STAGING) Ersatz Edamame E5", now.Add(-time.Hour), now.Add(89*24*time.Hour))
		}, certFlowIssued, certflow.KindStaging, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt, db, cookie := certFlowRouter(t, doctorFakeDNS{})
			tc.setup(rt, db)
			var st domainCertStatusResource
			if code := certFlowDo(t, rt, cookie, http.MethodGet, "/api/v1/domains/shop.example.dev/cert/status", nil, &st); code != http.StatusOK {
				t.Fatalf("status = %d", code)
			}
			if st.State != tc.state || st.Kind != tc.kind {
				t.Fatalf("state/kind = %q/%q, want %q/%q (%+v)", st.State, st.Kind, tc.state, tc.kind, st)
			}
			if !strings.Contains(st.Explanation, tc.explain) {
				t.Fatalf("explanation = %q, want it to contain %q", st.Explanation, tc.explain)
			}
			if tc.state == certFlowIssued && (st.NotAfter == nil || st.DaysLeft == nil || st.RenewalAt == nil || st.RenewalAt.After(*st.NotAfter)) {
				t.Fatalf("issued details missing: %+v", st)
			}
		})
	}
}

func enableACMEForTest(t *testing.T, db *store.DB, directory string) {
	t.Helper()
	s, err := db.GetIngressSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	s.ACMEEnabled, s.ACMEEmail, s.ACMEDirectoryURL = true, "me@shop.dev", directory
	if err := db.UpdateIngressSettings(context.Background(), s); err != nil {
		t.Fatal(err)
	}
}

func TestCertFlowListCarriesTruthBadge(t *testing.T) {
	rt, db, cookie := certFlowRouter(t, doctorFakeDNS{})
	enableACMEForTest(t, db, "")
	now := time.Now()
	seedCertIssuer(t, db, "R11", now.Add(-10*24*time.Hour), now.Add(80*24*time.Hour))
	var rows []certificateStatus
	if code := certFlowDo(t, rt, cookie, http.MethodGet, "/api/v1/certificates", nil, &rows); code != http.StatusOK || len(rows) != 1 {
		t.Fatalf("status = %d rows = %d", code, len(rows))
	}
	r := rows[0]
	if r.Kind != certflow.KindLetsEncrypt || r.DaysLeft < 79 || r.LastRenewal == nil || r.LastRenewal.Result != certflow.RenewalOK {
		t.Fatalf("row = %+v", r)
	}

	ingress.DefaultACMEFailures().Record(ingress.ACMEFailure{Identifier: "shop.example.dev", Error: "boom", Renewal: true, At: now})
	certFlowDo(t, rt, cookie, http.MethodGet, "/api/v1/certificates", nil, &rows)
	if rows[0].LastRenewal == nil || rows[0].LastRenewal.Result != certflow.RenewalFailed || rows[0].ExpiryClass != "incident" {
		t.Fatalf("failed renewal row = %+v", rows[0])
	}
}

func TestDoctorCertificateCheckThresholds(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name   string
		env    string
		left   time.Duration
		fail   bool
		status string
	}{
		{"far from expiry", "", 60 * 24 * time.Hour, false, doctorStatusOK},
		{"inside default 14 days", "", 10 * 24 * time.Hour, true, doctorStatusFail},
		{"env lowers the window", "5", 10 * 24 * time.Hour, false, doctorStatusOK},
		{"env raises the window", "40", 30 * 24 * time.Hour, true, doctorStatusFail},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.env != "" {
				t.Setenv(certflow.EnvFailDays, tc.env)
			}
			rt, db, _ := certFlowRouter(t, doctorFakeDNS{})
			enableACMEForTest(t, db, "")
			seedCertIssuer(t, db, "R11", now.Add(-60*24*time.Hour), now.Add(tc.left))
			got := rt.doctorCheckCertExpiry(context.Background())
			if got.Status != tc.status {
				t.Fatalf("status = %q, want %q (%s / %s)", got.Status, tc.status, got.Message, got.Evidence)
			}
		})
	}
}

func TestDoctorCertificateCheckFailedRenewalHasTypedFix(t *testing.T) {
	rt, db, _ := certFlowRouter(t, doctorFakeDNS{})
	enableACMEForTest(t, db, "")
	now := time.Now()
	seedCertIssuer(t, db, "R11", now.Add(-5*24*time.Hour), now.Add(85*24*time.Hour))
	ingress.DefaultACMEFailures().Record(ingress.ACMEFailure{Identifier: "shop.example.dev", Error: "rateLimited", Renewal: true, At: now})
	got := rt.doctorCheckCertExpiry(context.Background())
	if got.Status != doctorStatusFail || !strings.Contains(got.Evidence, "last renewal failed") {
		t.Fatalf("check = %+v", got)
	}
	fix := rt.certRenewFix()
	app, err := fix.Detect(context.Background())
	if err != nil || !app.Failing || !strings.Contains(app.Detail, "shop.example.dev") {
		t.Fatalf("fix detect = %+v err = %v", app, err)
	}
	if fix.Kind != "auto" || fix.ID != certsRenewFixID || fix.CheckID != "certificates" {
		t.Fatalf("fix = %+v", fix)
	}
}

func TestCertWatchdogScanBlocksOnRateLimitAndRenews(t *testing.T) {
	rt, db, _ := certFlowRouter(t, doctorFakeDNS{})
	enableACMEForTest(t, db, "")
	now := time.Now()
	seedCertIssuer(t, db, "R11", now.Add(-5*24*time.Hour), now.Add(85*24*time.Hour))
	if got, err := rt.watchdogScan(context.Background()); err != nil || len(got) != 0 {
		t.Fatalf("healthy cert scanned as %+v err %v", got, err)
	}
	ingress.DefaultACMEFailures().Record(ingress.ACMEFailure{Identifier: "shop.example.dev", Error: "boom", Renewal: true, At: now})
	got, err := rt.watchdogScan(context.Background())
	if err != nil || len(got) != 1 || got[0].Reason != "renewal_failed" || got[0].Expired || got[0].Blocked {
		t.Fatalf("scan = %+v err %v", got, err)
	}
	ingress.DefaultACMEFailures().Record(ingress.ACMEFailure{Identifier: "shop.example.dev", Error: "urn:acme:error:rateLimited", Renewal: true, At: now})
	got, _ = rt.watchdogScan(context.Background())
	if len(got) != 1 || !got[0].Blocked {
		t.Fatalf("rate limited scan = %+v", got)
	}
	if err := rt.watchdogRenew(context.Background(), certflow.Candidate{Domain: "shop.example.dev", Expired: true}); err != nil {
		t.Fatal(err)
	}
	rows, _ := rt.certTruthRows(context.Background(), time.Now())
	if len(rows) != 0 {
		t.Fatalf("an expired candidate's stored certificate must be cleared, rows = %d", len(rows))
	}
}

const (
	keyProd     = "acme-v02.api.letsencrypt.org-directory"
	keyStaging  = "acme-staging-v02.api.letsencrypt.org-directory"
	keyInternal = "local"
)

type certCell struct {
	name     string
	key, iss string
	expired  bool
}

var certCells = []certCell{
	{"internal", keyInternal, "Caddy Local Authority - ECC Intermediate", false},
	{"staging", keyStaging, "(STAGING) Ersatz Edamame E5", false},
	{"letsencrypt valid", keyProd, "R11", false},
	{"letsencrypt expired", keyProd, "R11", true},
}

func (c certCell) seed(t *testing.T, db *store.DB) {
	now := time.Now()
	if c.expired {
		seedCertAt(t, db, c.key, c.iss, now.Add(-100*24*time.Hour), now.Add(-time.Hour))
		return
	}
	seedCertAt(t, db, c.key, c.iss, now.Add(-10*24*time.Hour), now.Add(80*24*time.Hour))
}

func storedKinds(t *testing.T, rt *Router) []certflow.Kind {
	t.Helper()
	rows, err := rt.certTruthRows(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var out []certflow.Kind
	for _, r := range rows {
		out = append(out, r.Kind)
	}
	return out
}

func TestApplyPublicCertMatrix(t *testing.T) {
	tests := []struct {
		cell    certCell
		staging bool
		want    string
	}{
		{certCells[0], true, "replaced"},
		{certCells[0], false, "replaced"},
		{certCells[1], true, "kept"},
		{certCells[1], false, "replaced"},
		{certCells[2], true, "blocked"},
		{certCells[2], false, "kept"},
		{certCells[3], true, "replaced"},
		{certCells[3], false, "kept"},
	}
	for _, tc := range tests {
		name := tc.cell.name + " requesting production"
		if tc.staging {
			name = tc.cell.name + " requesting staging"
		}
		t.Run(name, func(t *testing.T) {
			rt, db, _ := certFlowRouter(t, doctorFakeDNS{})
			tc.cell.seed(t, db)
			err := rt.applyPublicCert(context.Background(), "shop.example.dev", "me@shop.dev", tc.staging)
			s, _ := db.GetIngressSettings(context.Background())
			kinds := storedKinds(t, rt)
			switch tc.want {
			case "blocked":
				var rep *certflow.ReplaceError
				if !errors.As(err, &rep) {
					t.Fatalf("err = %v, want a ReplaceError", err)
				}
				if s.ACMEEnabled || len(kinds) != 1 {
					t.Fatalf("blocked must change nothing: acme=%v kinds=%v", s.ACMEEnabled, kinds)
				}
				if !strings.Contains(rep.Error(), "Next step") {
					t.Fatalf("message = %q", rep.Error())
				}
			case "kept":
				if err != nil || len(kinds) != 1 || !s.ACMEEnabled {
					t.Fatalf("err = %v kinds = %v", err, kinds)
				}
			case "replaced":
				if err != nil || len(kinds) != 0 || !s.ACMEEnabled {
					t.Fatalf("err = %v kinds = %v", err, kinds)
				}
			}
		})
	}
}

func TestApplyPublicCertStagingBlockedByAnotherDomainsWorkingCert(t *testing.T) {
	rt, db, _ := certFlowRouter(t, doctorFakeDNS{})
	now := time.Now()
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "other.dev"}, DNSNames: []string{"other.dev"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(80 * 24 * time.Hour)}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "R11"}}, &priv.PublicKey, priv)
	if err := db.SaveCertStorageValue(context.Background(), "certificates/"+keyProd+"/other.dev/other.dev.crt", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})); err != nil {
		t.Fatal(err)
	}
	var rep *certflow.ReplaceError
	if err := rt.applyPublicCert(context.Background(), "shop.example.dev", "me@shop.dev", true); !errors.As(err, &rep) || rep.Domain != "other.dev" {
		t.Fatalf("err = %v", err)
	}
}

func TestCertFlowStagingBlockedHTTP(t *testing.T) {
	rt, db, cookie := certFlowRouter(t, doctorFakeDNS{ips: []string{certFlowServerIP}})
	certCells[2].seed(t, db)
	var pre certflow.Result
	certFlowDo(t, rt, cookie, http.MethodGet, "/api/v1/domains/shop.example.dev/cert/preflight?email=me@shop.dev&staging=true", nil, &pre)
	found := false
	for _, it := range pre.Items {
		if it.Code == certflow.CodeReplaces && it.State == certflow.StateFail && strings.Contains(it.Detail, "Next step") && it.Fix != "" {
			found = true
		}
	}
	if pre.Ready || !found {
		t.Fatalf("preflight = %+v", pre)
	}
	var blocked getCertBlocked
	if code := certFlowDo(t, rt, cookie, http.MethodPost, "/api/v1/domains/shop.example.dev/cert", map[string]any{"email": "me@shop.dev", "staging": true}, &blocked); code != http.StatusConflict || blocked.Error == "" {
		t.Fatalf("status = %d body = %+v", code, blocked)
	}
	if s, _ := db.GetIngressSettings(context.Background()); s.ACMEEnabled {
		t.Fatal("settings changed")
	}
	var prod certflow.Result
	certFlowDo(t, rt, cookie, http.MethodGet, "/api/v1/domains/shop.example.dev/cert/preflight?email=me@shop.dev", nil, &prod)
	if !prod.Ready {
		t.Fatalf("production over a working cert stays allowed: %+v", prod.Items)
	}
}
