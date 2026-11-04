package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// brokenLedger fails every ledger owner lookup so the fail-closed path runs.
type brokenLedger struct{ *store.DB }

func (brokenLedger) GetStackResourceOwner(context.Context, string, string) (store.StackLedgerEntry, bool, error) {
	return store.StackLedgerEntry{}, false, errors.New("ledger offline")
}

func TestStackLockRuntimeSetting(t *testing.T) {
	rt, db, cookie, _ := stackLockFixture(t, "lock")
	stack, err := db.GetStackByName(t.Context(), "shop-prod")
	if err != nil {
		t.Fatal(err)
	}
	entry := store.StackLedgerEntry{StackID: stack.ID, Kind: "Setting", Key: "Setting/owned.key", LiveID: "owned.key", SinceRevision: 1}
	if err := db.UpsertStackResource(t.Context(), entry); err != nil {
		t.Fatal(err)
	}
	const owned = "/api/v1/settings/runtime/owned.key"
	for _, m := range []string{http.MethodPut, http.MethodDelete} {
		code, out := stackLockCall(t, rt, authedRequest(t, cookie, m, owned, `{"value":1}`))
		if code != http.StatusConflict || out.Code != errCodeStackLocked || !strings.Contains(out.Error, "shop-prod") {
			t.Errorf("%s owned = %d %+v, want 409 stack_locked", m, code, out)
		}
		req := authedRequest(t, cookie, m, owned, `{"value":1}`)
		req.Header.Set(headerOverrideStackLock, "shop-prod")
		if code, out := stackLockCall(t, rt, req); code == http.StatusConflict || out.Code == errCodeStackLocked {
			t.Errorf("override %s = %d %+v", m, code, out)
		}
		if code, out := stackLockCall(t, rt, authedRequest(t, cookie, m, "/api/v1/settings/runtime/free.key", `{"value":1}`)); out.Code == errCodeStackLocked || code == http.StatusConflict {
			t.Errorf("unowned %s = %d %+v", m, code, out)
		}
	}

	rt.policies = brokenLedger{db}
	code, out := stackLockCall(t, rt, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/runtime/free.key", `{"value":1}`))
	if code != http.StatusServiceUnavailable || out.Code != errCodeStackLockUnavailable {
		t.Errorf("setting write on ledger error = %d %+v, want 503 fail closed", code, out)
	}
	if code, out := stackLockCall(t, rt, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web", `{}`)); code == http.StatusServiceUnavailable || out.Code == errCodeStackLockUnavailable {
		t.Errorf("app write on ledger error = %d %+v, want fail open", code, out)
	}
}
