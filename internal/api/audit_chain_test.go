package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestVerifyAuditChainEndpointReportsFirstBrokenLink(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	verify := func() auditChainResource {
		t.Helper()
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/audit-log/verify", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
		}
		var out auditChainResource
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return out
	}

	entry := store.AuditEntry{ID: "aud_chain_1", ActorType: "session", ActorID: "u", ActorName: "n", Ability: "write", Method: "POST", Path: "/p", StatusCode: 200, CreatedAt: store.FormatAuditTime(time.Now())}
	if err := db.SaveAuditEntry(ctx, entry); err != nil {
		t.Fatalf("SaveAuditEntry: %v", err)
	}
	if got := verify(); !got.OK || got.Chained == 0 {
		t.Fatalf("fresh chain = %+v, want ok with chained rows", got)
	}

	if _, err := db.ExecContext(ctx, `UPDATE audit_log SET path = '/tampered' WHERE seq = 1`); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	got := verify()
	if got.OK || got.FirstBroken == nil || got.FirstBroken.Seq != 1 {
		t.Fatalf("tampered chain = %+v, want first broken at seq 1", got)
	}
}
