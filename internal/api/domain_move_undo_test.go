package api

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const moveCertKey = "certificates/acme-v02/a.example.com/a.example.com.crt"

func moveRouter(t *testing.T) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	for _, svc := range []store.DesiredService{
		{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"a.example.com"}},
		{Name: "api", Image: "img:v1", Port: 80},
	} {
		if err := db.SaveDesiredService(context.Background(), svc); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SaveCertStorageValue(context.Background(), moveCertKey, []byte("pem")); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveCertLedger(context.Background(), store.CertLedgerEntry{Domain: "a.example.com", Kind: "issue", At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	return rt, db, cookie
}

func doMove(t *testing.T, rt *Router, cookie *http.Cookie) {
	t.Helper()
	rec := sdReq(t, rt, cookie, http.MethodPost, "/api/v1/domains/move", `{"domain":"a.example.com","from":"web","to":"api","confirm":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("move = %d %s", rec.Code, rec.Body.String())
	}
}

func domainsOf(t *testing.T, db *store.DB, app string) []string {
	t.Helper()
	svc, err := db.GetDesiredService(context.Background(), app)
	if err != nil {
		t.Fatal(err)
	}
	return svc.Domains
}

func TestMoveDomainKeepsCertificateAndFlipsRouteAtOnce(t *testing.T) {
	rt, db, cookie := moveRouter(t)
	doMove(t, rt, cookie)
	got, err := db.GetCertStorageValue(context.Background(), moveCertKey)
	if err != nil || string(got.Value) != "pem" {
		t.Fatalf("certificate after move = %v, %v", got, err)
	}
	if rows, _ := db.LoadCertLedger(context.Background(), time.Now(), time.Hour); len(rows) != 1 {
		t.Fatalf("cert ledger rows = %d, want the original only", len(rows))
	}
	// One transaction: the host is on the target and gone from the source, never on neither.
	if !slices.Contains(domainsOf(t, db, "api"), "a.example.com") || len(domainsOf(t, db, "web")) != 0 {
		t.Fatalf("api = %v web = %v", domainsOf(t, db, "api"), domainsOf(t, db, "web"))
	}
}

func TestMoveDomainIsRecordedAndRollsBack(t *testing.T) {
	rt, db, cookie := moveRouter(t)
	doMove(t, rt, cookie)
	base := "/api/v1/domains/a.example.com/changes"
	var list []domainChange
	changesCall(t, rt, cookie, http.MethodGet, base, &list)
	if len(list) != 1 || list[0].Kind != "domain_move" || !list[0].CanRollback || !strings.Contains(list[0].Summary[0].Description, "web") {
		t.Fatalf("changes = %+v", list)
	}
	var out rollbackResponse
	if code := changesCall(t, rt, cookie, http.MethodPost, base+"/"+list[0].RunID+"/rollback", &out); code != http.StatusOK {
		t.Fatalf("rollback = %d", code)
	}
	if out.Outcomes[0].Status != rollbackReverted {
		t.Fatalf("outcomes = %+v", out.Outcomes)
	}
	if !slices.Contains(domainsOf(t, db, "web"), "a.example.com") || len(domainsOf(t, db, "api")) != 0 {
		t.Fatalf("after rollback web = %v api = %v", domainsOf(t, db, "web"), domainsOf(t, db, "api"))
	}
	if _, err := db.GetCertStorageValue(context.Background(), moveCertKey); err != nil {
		t.Fatalf("certificate lost on rollback: %v", err)
	}
}

func TestMoveRollbackRefusedWhenTargetChanged(t *testing.T) {
	rt, db, cookie := moveRouter(t)
	doMove(t, rt, cookie)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "api", Image: "img:v1", Port: 80, Domains: []string{}}); err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/domains/a.example.com/changes"
	var list []domainChange
	changesCall(t, rt, cookie, http.MethodGet, base, &list)
	var out rollbackResponse
	changesCall(t, rt, cookie, http.MethodPost, base+"/"+list[0].RunID+"/rollback", &out)
	if len(out.Outcomes) != 1 || out.Outcomes[0].Status != rollbackSkipped {
		t.Fatalf("outcomes = %+v, want skipped", out.Outcomes)
	}
	if slices.Contains(domainsOf(t, db, "web"), "a.example.com") {
		t.Fatal("rollback moved the domain although the target no longer served it")
	}
}
