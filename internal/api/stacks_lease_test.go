package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestStackApplyRenewsLeasePastTTL(t *testing.T) {
	t.Setenv("APP_STACK_LOCK_TTL", "120ms")
	rt, db, cookie := stackFixture(t)
	stackBeforeApply = func() { time.Sleep(300 * time.Millisecond) }
	t.Cleanup(func() { stackBeforeApply = nil })
	if code, out := stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil); code != http.StatusOK || out.Revision == nil || out.Revision.Status != "applied" {
		t.Fatalf("apply past ttl = %d %+v", code, out)
	}
	if _, err := db.GetDesiredService(t.Context(), "web"); err != nil {
		t.Fatalf("app not applied: %v", err)
	}
}

func TestStackApplyAbortsWhenLeaseLost(t *testing.T) {
	t.Setenv("APP_STACK_LOCK_TTL", "120ms")
	rt, db, cookie := stackFixture(t)
	id := mustStackID(t, db)
	stackBeforeApply = func() {
		if err := db.ForceReleaseStackLock(t.Context(), id); err != nil {
			t.Error(err)
		}
		if _, err := db.AcquireStackLock(t.Context(), id, "intruder", 2, time.Minute, time.Now()); err != nil {
			t.Error(err)
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Cleanup(func() { stackBeforeApply = nil })
	req := authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/shop-prod/apply", stackBody(t, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil))
	var body struct {
		Error string `json:"error"`
	}
	if code := iacCall(t, rt, req, &body); code != http.StatusConflict || !strings.Contains(body.Error, "lease was lost") {
		t.Fatalf("apply after lease loss = %d %q", code, body.Error)
	}
	rev, err := db.GetStackRevision(t.Context(), id, 1)
	if err != nil || rev.Status != store.StackRevFailed || !strings.Contains(string(rev.ResultJSON), "lease was lost") {
		t.Fatalf("revision = %+v %v", rev.Status, err)
	}
	if _, err := db.GetDesiredService(t.Context(), "web"); err == nil {
		t.Fatal("apply ran after the lease was lost")
	}
}
