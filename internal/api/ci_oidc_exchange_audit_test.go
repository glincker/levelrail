package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestCIPublicAddrBlocksTranslationAndReservedRanges(t *testing.T) {
	for _, addr := range []string{"64:ff9b::7f00:1", "2002:7f00:1::1", "198.18.0.1", "198.19.255.1", "240.0.0.1", "255.255.255.255", "192.0.0.8", "100.127.0.1"} {
		if ciPublicAddr(netip.MustParseAddr(addr)) {
			t.Errorf("ciPublicAddr(%s) = true, want blocked", addr)
		}
	}
}

func TestCIOIDCMaxTokenAgeDefaultsToOneHour(t *testing.T) {
	t.Setenv(envCIOIDCMaxTokenAge, "")
	if got := ciOIDCConfigFromEnv().MaxTokenAge; got != time.Hour {
		t.Fatalf("unset max token age = %v, want 1h", got)
	}
	t.Setenv(envCIOIDCMaxTokenAge, "not-a-duration")
	if got := ciOIDCConfigFromEnv().MaxTokenAge; got != time.Hour {
		t.Fatalf("unparsable max token age = %v, want the 1h default", got)
	}
	t.Setenv(envCIOIDCMaxTokenAge, "off")
	if got := ciOIDCConfigFromEnv().MaxTokenAge; got != 0 {
		t.Fatalf("off = %v, want 0", got)
	}
}

func TestCIOIDCExchangeWritesWindowedAuditRows(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "audited")
	if err := db.SetDashboardURL(context.Background(), "https://cp.example.com"); err != nil {
		t.Fatal(err)
	}
	ver := &fakeCIVerifier{claims: CIClaims{Repository: "acme/site", Ref: "refs/heads/main", Subject: "repo:acme/site:ref:refs/heads/main"}}
	rt.SetCIIDTokenVerifier(ver)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/audited/ci/trusts", `{"repository":"acme/site","ref_pattern":"refs/heads/main"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d", rec.Code)
	}
	if got := ciExchange(t, rt, "audited").Code; got != http.StatusOK {
		t.Fatalf("exchange = %d", got)
	}
	ver.claims.Ref = "refs/heads/dev"
	for range 5 {
		if got := ciExchange(t, rt, "audited").Code; got != http.StatusForbidden {
			t.Fatalf("untrusted ref = %d", got)
		}
	}
	entries, err := db.ListAuditEntries(context.Background(), 200, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, e := range entries {
		if e.Action == auditActionCIExchangeIssued || e.Action == auditActionCIExchangeRefused {
			counts[e.Action]++
			if strings.Contains(e.Detail, "repo:acme/site") {
				t.Errorf("audit row carries the raw subject: %s", e.Detail)
			}
			if !strings.Contains(e.Detail, `"app":"audited"`) || !strings.Contains(e.Detail, `"provider":"github"`) {
				t.Errorf("audit row lacks app or provider: %s", e.Detail)
			}
		}
	}
	if counts[auditActionCIExchangeIssued] != 1 || counts[auditActionCIExchangeRefused] != 1 {
		t.Fatalf("audit rows = %v, want one issued and one windowed refusal", counts)
	}
}

func TestCIExchangeAuditWindowCountsSuppressed(t *testing.T) {
	w := newCIExchangeAuditWindow(time.Minute, 2)
	now := time.Now()
	if ok, _ := w.admit("a", now); !ok {
		t.Fatal("first refusal not admitted")
	}
	for range 3 {
		if ok, _ := w.admit("a", now.Add(time.Second)); ok {
			t.Fatal("refusal inside the window admitted")
		}
	}
	ok, suppressed := w.admit("a", now.Add(2*time.Minute))
	if !ok || suppressed != 3 {
		t.Fatalf("after window: ok = %v suppressed = %d, want true 3", ok, suppressed)
	}
	w.admit("b", now)
	if ok, _ := w.admit("c", now); ok {
		t.Fatal("window grew past its key cap")
	}
}
