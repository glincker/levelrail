package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeKeyRotator struct {
	status    secrets.DEKStatus
	rotated   []string
	allCalls  int
	rotateErr error
}

func (f *fakeKeyRotator) RotateOne(_ context.Context, name string) (secrets.DEKRotateResult, error) {
	if f.rotateErr != nil {
		return secrets.DEKRotateResult{}, f.rotateErr
	}
	f.rotated = append(f.rotated, name)
	return secrets.DEKRotateResult{Service: name, Values: 3, RotatedAt: time.Now().UTC()}, nil
}

func (f *fakeKeyRotator) RotateAll(context.Context, string) (secrets.DEKRotateAllResult, error) {
	f.allCalls++
	return secrets.DEKRotateAllResult{RunID: "dek_x", Total: 2, Rotated: 2, Complete: true}, nil
}

func (f *fakeKeyRotator) Status(context.Context) (secrets.DEKStatus, error) { return f.status, nil }

type fakeMasterAge struct{ at time.Time }

func (f fakeMasterAge) RotateMasterKey(context.Context, string) (time.Time, error) {
	return time.Time{}, nil
}

func (f fakeMasterAge) GetMasterKeyRotatedAt(context.Context) (time.Time, bool, error) {
	return f.at, !f.at.IsZero(), nil
}

func keysStepUp(t *testing.T, rt *Router, cookie *http.Cookie) {
	t.Helper()
	body := `{"password":"` + testAdminPassword + `","action":"manage","app":"` + encryptionKeysStepUpApp + `"}`
	if rec := serveMobile(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/step-up", body)); rec.Code != http.StatusOK {
		t.Fatalf("step-up = %d %s", rec.Code, rec.Body.String())
	}
}

func TestEncryptionKeys_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := serveMobile(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/encryption-keys", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestEncryptionKeys_StatusPolicyAndHints(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	fr := &fakeKeyRotator{status: secrets.DEKStatus{Services: []secrets.ServiceKeyStatus{
		{Service: "web", AgeDays: 120, ValueCount: 4},
		{Service: "api", AgeDays: 130, ValueCount: 1},
		{Service: "fresh", AgeDays: 2, ValueCount: 1},
	}}}
	rt.masterKeyRotator = fakeMasterAge{at: time.Now().Add(-200 * 24 * time.Hour)}
	rt.SetEncryptionKeys(fr, db)

	get := func() encryptionKeysResource {
		rec := serveMobile(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/encryption-keys", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET = %d %s", rec.Code, rec.Body.String())
		}
		var res encryptionKeysResource
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
		return res
	}

	if res := get(); res.Policy.MaxKeyAgeDays != store.DefaultMaxKeyAgeDays {
		t.Fatalf("unset policy = %d days, want the default", res.Policy.MaxKeyAgeDays)
	}
	if err := db.SaveSecretKeyPolicy(context.Background(), store.SecretKeyPolicy{RemindEveryDays: 7}, time.Now()); err != nil {
		t.Fatal(err)
	}
	res := get()
	if len(res.Apps) != 3 || res.Policy.MaxKeyAgeDays != 0 || res.MasterKey.AgeDays == nil {
		t.Fatalf("status = %+v", res)
	}
	if len(res.Hints) != 2 || res.Hints[0].Fix.Kind != keyFixEnablePolicy || res.Hints[1].Fix.Kind != keyFixRotateMasterKey || res.Hints[1].Severity != "info" {
		t.Fatalf("hints with policy off = %+v", res.Hints)
	}

	rec := serveMobile(rt, authedRequest(t, cookie, http.MethodPut, "/api/v1/system/encryption-keys/policy", `{"max_key_age_days":90,"remind_every_days":3}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT policy = %d %s", rec.Code, rec.Body.String())
	}
	res = get()
	kinds := map[string]int{}
	for _, h := range res.Hints {
		kinds[h.Fix.Kind]++
	}
	if res.Policy.MaxKeyAgeDays != 90 || kinds[keyFixRotateMasterKey] != 1 || kinds[keyFixRotateApp] != 2 || kinds[keyFixRotateAll] != 1 {
		t.Fatalf("policy %+v hints %+v", res.Policy, res.Hints)
	}
}

func TestEncryptionKeys_HintsNameOverdueApps(t *testing.T) {
	res := encryptionKeysResource{Apps: []appKeyResource{{App: "web", Overdue: true}, {App: "api", Overdue: true}, {App: "ok"}}}
	hints := encryptionKeyHints(res, 90)
	kinds := map[string]int{}
	for _, h := range hints {
		kinds[h.Fix.Kind]++
	}
	if kinds[keyFixRotateApp] != 2 || kinds[keyFixRotateAll] != 1 {
		t.Fatalf("hints = %+v", hints)
	}
}

func TestEncryptionKeys_PolicyOffOffersOneClickFix(t *testing.T) {
	off := encryptionKeyHints(encryptionKeysResource{}, 0)
	if len(off) != 1 || off[0].Fix.Kind != keyFixEnablePolicy {
		t.Fatalf("policy off hints = %+v, want one enable_key_policy fix", off)
	}
	if on := encryptionKeyHints(encryptionKeysResource{}, 90); len(on) != 0 {
		t.Fatalf("policy on hints = %+v, want none", on)
	}
}

func TestEncryptionKeys_PolicyValidation(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rt.SetEncryptionKeys(&fakeKeyRotator{}, db)
	for _, body := range []string{`{"max_key_age_days":-1}`, `{"max_key_age_days":99999}`, `{"remind_every_days":0}`} {
		rec := serveMobile(rt, authedRequest(t, cookie, http.MethodPut, "/api/v1/system/encryption-keys/policy", body))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d", body, rec.Code)
		}
	}
}

func TestEncryptionKeys_RotateNeedsStepUpAndRoot(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	fr := &fakeKeyRotator{}
	rt.SetEncryptionKeys(fr, db)
	post := func(c *http.Cookie, body string) (int, string) {
		rec := serveMobile(rt, authedRequest(t, c, http.MethodPost, "/api/v1/system/encryption-keys/rotate", body))
		return rec.Code, rec.Body.String()
	}

	if code, body := post(cookie, `{"service":"web"}`); code != http.StatusForbidden || !strings.Contains(body, errCodeStepUpRequired) {
		t.Fatalf("no step-up = %d %s", code, body)
	}
	if len(fr.rotated) != 0 {
		t.Fatal("rotated without step-up")
	}
	if code, _ := post(cookie, `{}`); code != http.StatusBadRequest {
		t.Fatalf("empty body = %d", code)
	}
	if code, _ := post(cookie, `{"service":"web","all":true}`); code != http.StatusBadRequest {
		t.Fatalf("both = %d", code)
	}

	reader := storeUserWithAbilitiesForTest(t, db, "reader@example.com", []string{AbilityRead})
	if code, _ := post(sessionCookieForTest(t, rt, reader.ID), `{"service":"web"}`); code != http.StatusForbidden {
		t.Fatalf("reader = %d", code)
	}

	keysStepUp(t, rt, cookie)
	code, body := post(cookie, `{"service":"web"}`)
	if code != http.StatusOK || len(fr.rotated) != 1 || fr.rotated[0] != "web" || !strings.Contains(body, `"values":3`) {
		t.Fatalf("rotate one = %d %s", code, body)
	}
	if code, body := post(cookie, `{"all":true}`); code != http.StatusOK || fr.allCalls != 1 || !strings.Contains(body, `"complete":true`) {
		t.Fatalf("rotate all = %d %s", code, body)
	}
}

func TestEncryptionKeys_RotateErrorsAreGeneric(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	fr := &fakeKeyRotator{}
	rt.SetEncryptionKeys(fr, db)
	keysStepUp(t, rt, cookie)

	tests := []struct {
		err  error
		want int
	}{
		{secrets.ErrDEKRotationBusy, http.StatusConflict},
		{store.ErrServiceDEKNotFound, http.StatusNotFound},
		{store.ErrDEKRotationConflict, http.StatusConflict},
		{secrets.ErrDEKUnwrap, http.StatusConflict},
	}
	for _, tc := range tests {
		fr.rotateErr = tc.err
		rec := serveMobile(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/encryption-keys/rotate", `{"service":"web"}`))
		if rec.Code != tc.want {
			t.Errorf("%v = %d", tc.err, rec.Code)
		}
	}
}
