package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func expireDeviceRowForTest(t *testing.T, db *store.DB, userCode string, expiresAt time.Time) {
	t.Helper()
	code := strings.ReplaceAll(userCode, "-", "")
	res, err := db.ExecContext(context.Background(), `UPDATE theauth_device_codes SET expires_at = ? WHERE user_code = ?`, expiresAt.UTC().UnixMicro(), code)
	if err != nil {
		t.Fatalf("expire row: %v", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		t.Fatalf("expire row: %d rows changed for %q", n, userCode)
	}
}

func deviceRequestIDForTest(t *testing.T, rt *Router, userCode string) string {
	t.Helper()
	d, err := rt.authLib.device.DeviceRequestByCode(context.Background(), userCode, time.Now())
	if err != nil {
		t.Fatalf("lookup request: %v", err)
	}
	return d.ID
}

func auditByAction(t *testing.T, db *store.DB, action string) []store.AuditEntry {
	t.Helper()
	entries, err := db.ListAuditEntries(context.Background(), 200, nil, store.AuditEntryFilter{Action: action})
	if err != nil {
		t.Fatalf("ListAuditEntries: %v", err)
	}
	return entries
}

func assertNoSecret(t *testing.T, e store.AuditEntry, secrets ...string) {
	t.Helper()
	for _, s := range secrets {
		for _, field := range []string{e.Path, e.ActorID, e.ActorName, e.Ability, e.Method} {
			if s != "" && strings.Contains(field, s) {
				t.Errorf("audit entry leaked %q: %+v", s, e)
			}
		}
	}
}

func TestSweepDeviceLoginExpiry_ExactlyOnce(t *testing.T) {
	tests := []struct {
		name      string
		nowOffset time.Duration
		wantFirst int
	}{
		{"well before expiry", -time.Minute, 0},
		{"one microsecond before expiry", -time.Microsecond, 0},
		{"exactly at expiry", 0, 1},
		{"long after expiry", 24 * time.Hour, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			started := startDeviceAuth(t, rt, "laptop")
			id := deviceRequestIDForTest(t, rt, started.UserCode)
			req, err := rt.authLib.device.DeviceRequestByID(context.Background(), id, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			now := req.ExpiresAt.Add(tt.nowOffset)

			got, err := rt.SweepDeviceLoginExpiry(context.Background(), now)
			if err != nil || got != tt.wantFirst {
				t.Fatalf("first sweep = %d, %v, want %d", got, err, tt.wantFirst)
			}
			again, err := rt.SweepDeviceLoginExpiry(context.Background(), now)
			if err != nil || again != 0 {
				t.Fatalf("second sweep = %d, %v, want 0", again, err)
			}
			entries := auditByAction(t, db, store.AuditActionDeviceLoginExpired)
			if len(entries) != tt.wantFirst {
				t.Fatalf("expired audit entries = %d, want %d", len(entries), tt.wantFirst)
			}
			for _, e := range entries {
				if e.ActorType != auditActorSystem || e.Path != store.DeviceLoginAuditPath(id) {
					t.Errorf("entry = %+v, want system actor and request path", e)
				}
				assertNoSecret(t, e, started.UserCode, started.DeviceCode, strings.ReplaceAll(started.UserCode, "-", ""))
			}
		})
	}
}

func TestSweepDeviceLoginExpiry_ConcurrentAndRestart(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db)
	started := startDeviceAuth(t, rt, "laptop")
	expireDeviceRowForTest(t, db, started.UserCode, time.Now().Add(-time.Minute))

	var claimed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := rt.SweepDeviceLoginExpiry(context.Background(), time.Now())
			if err != nil {
				t.Errorf("sweep: %v", err)
			}
			claimed.Add(int64(n))
		}()
	}
	wg.Wait()
	if claimed.Load() != 1 {
		t.Fatalf("claimed across racing sweeps = %d, want 1", claimed.Load())
	}

	restarted := NewRouter(nil, testBrand(), db)
	if n, err := restarted.SweepDeviceLoginExpiry(context.Background(), time.Now()); err != nil || n != 0 {
		t.Errorf("sweep after restart = %d, %v, want 0", n, err)
	}
	if got := len(auditByAction(t, db, store.AuditActionDeviceLoginExpired)); got != 1 {
		t.Errorf("expired entries = %d, want 1", got)
	}
}

func TestDeviceExpiry_PollAfterExpiryDoesNotDoubleAudit(t *testing.T) {
	rt, db := newTestRouter(t)
	started := startDeviceAuth(t, rt, "laptop")
	expireDeviceRowForTest(t, db, started.UserCode, time.Now().Add(-time.Minute))

	for i := 0; i < 2; i++ {
		rec := pollDeviceToken(t, rt, started.DeviceCode)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "expired_token") {
			t.Fatalf("poll %d = %d %s, want 400 expired_token", i, rec.Code, rec.Body.String())
		}
	}
	if n, err := rt.SweepDeviceLoginExpiry(context.Background(), time.Now()); err != nil || n != 0 {
		t.Errorf("sweep after poll claimed %d, %v, want 0", n, err)
	}
	if got := len(auditByAction(t, db, store.AuditActionDeviceLoginExpired)); got != 1 {
		t.Errorf("expired entries = %d, want exactly 1", got)
	}
}

type activityBody struct {
	Items []deviceActivityItem `json:"items"`
}

func getActivity(t *testing.T, rt *Router, req *http.Request) activityBody {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("activity status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out activityBody
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode activity: %v", err)
	}
	return out
}

func dismissItem(t *testing.T, rt *Router, req *http.Request) int {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec.Code
}

func TestDeviceActivity_StatesAndPerUserDismissal(t *testing.T) {
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	reader := storeUserWithAbilitiesForTest(t, db, "reader@example.com", []string{AbilityRead})
	readerCookie := sessionCookieForTest(t, rt, reader.ID)

	approved := startDeviceAuth(t, rt, "approved-box")
	denied := startDeviceAuth(t, rt, "denied-box")
	expired := startDeviceAuth(t, rt, "expired-box")
	waiting := startDeviceAuth(t, rt, "waiting-box")
	for _, step := range []struct{ code, action string }{{approved.UserCode, "approve"}, {denied.UserCode, "deny"}} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, admin, http.MethodPost, "/api/v1/auth/device/"+step.code+"/"+step.action, ""))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s = %d", step.action, rec.Code)
		}
	}
	expireDeviceRowForTest(t, db, expired.UserCode, time.Now().Add(-time.Minute))

	list := func(c *http.Cookie) map[string]deviceActivityItem {
		out := map[string]deviceActivityItem{}
		for _, it := range getActivity(t, rt, authedRequest(t, c, http.MethodGet, "/api/v1/auth/device/activity", "")).Items {
			out[it.ClientName] = it
		}
		return out
	}
	got := list(admin)
	wantStates := map[string]string{
		"approved-box": store.DeviceLoginStateApproved, "denied-box": store.DeviceLoginStateDenied,
		"expired-box": store.DeviceLoginStateExpired, "waiting-box": store.DeviceLoginStateWaiting,
	}
	for name, state := range wantStates {
		if got[name].State != state {
			t.Errorf("%s state = %q, want %q", name, got[name].State, state)
		}
	}
	if got["waiting-box"].Dismissible || !got["expired-box"].Dismissible {
		t.Errorf("dismissible: waiting=%v expired=%v", got["waiting-box"].Dismissible, got["expired-box"].Dismissible)
	}
	raw, _ := json.Marshal(got)
	for _, secret := range []string{waiting.UserCode, waiting.DeviceCode, expired.UserCode, expired.DeviceCode} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("activity leaked a code %q", secret)
		}
	}

	dismiss := func(c *http.Cookie, key string) int {
		return dismissItem(t, rt, authedRequest(t, c, http.MethodPost, "/api/v1/attention/dismiss", `{"item_key":"`+key+`"}`))
	}
	if code := dismiss(admin, got["waiting-box"].ItemKey); code != http.StatusConflict {
		t.Errorf("dismiss waiting = %d, want 409", code)
	}
	stale := store.DeviceLoginItemKey(got["expired-box"].ID, store.DeviceLoginStateDenied)
	if code := dismiss(admin, stale); code != http.StatusConflict {
		t.Errorf("dismiss with a stale state = %d, want 409", code)
	}
	if code := dismiss(admin, "device_login:nope:expired"); code != http.StatusNotFound {
		t.Errorf("dismiss unknown = %d, want 404", code)
	}
	if code := dismiss(admin, "something-else"); code != http.StatusBadRequest {
		t.Errorf("dismiss malformed = %d, want 400", code)
	}
	key := got["expired-box"].ItemKey
	for i := 0; i < 2; i++ {
		if code := dismiss(admin, key); code != http.StatusNoContent {
			t.Fatalf("dismiss %d = %d, want 204", i, code)
		}
	}
	if !list(admin)["expired-box"].Dismissed {
		t.Error("dismissal did not persist for the dismissing user")
	}
	if list(readerCookie)["expired-box"].Dismissed {
		t.Error("another user sees the first user's dismissal")
	}
	if got := len(auditByAction(t, db, store.AuditActionDeviceLoginDismissed)); got != 1 {
		t.Errorf("dismiss audit entries = %d, want 1 (idempotent)", got)
	}

	fresh := startDeviceAuth(t, rt, "fresh-box")
	expireDeviceRowForTest(t, db, fresh.UserCode, time.Now().Add(-time.Second))
	if list(admin)["fresh-box"].Dismissed {
		t.Error("a new request inherited an earlier dismissal")
	}

	tok := mintTokenAs(t, rt, admin, "agent")
	if code := dismissItem(t, rt, bearerRequest(http.MethodPost, "/api/v1/attention/dismiss", `{"item_key":"`+key+`"}`, tok.Token)); code != http.StatusUnauthorized {
		t.Errorf("dismiss with an API token = %d, want 401", code)
	}
	viaToken := getActivity(t, rt, bearerRequest(http.MethodGet, "/api/v1/auth/device/activity", "", tok.Token))
	if len(viaToken.Items) == 0 {
		t.Error("a read token should see device activity")
	}
}

func TestAuditLog_DeviceLoginFamilyFilter(t *testing.T) {
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	a := startDeviceAuth(t, rt, "a")
	b := startDeviceAuth(t, rt, "b")
	c := startDeviceAuth(t, rt, "c")
	post := func(path string) {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, admin, http.MethodPost, path, ""))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s = %d", path, rec.Code)
		}
	}
	post("/api/v1/auth/device/" + a.UserCode + "/approve")
	post("/api/v1/auth/device/" + b.UserCode + "/deny")
	expireDeviceRowForTest(t, db, c.UserCode, time.Now().Add(-time.Minute))
	if _, err := rt.SweepDeviceLoginExpiry(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	id := deviceRequestIDForTest(t, rt, c.UserCode)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, admin, http.MethodPost, "/api/v1/attention/dismiss", `{"item_key":"`+store.DeviceLoginItemKey(id, store.DeviceLoginStateExpired)+`"}`))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("dismiss = %d", rec.Code)
	}

	fetch := func(q string) []auditLogEntryResource {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, admin, http.MethodGet, "/api/v1/audit-log?"+q, ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("audit-log %s = %d", q, rec.Code)
		}
		var out []auditLogEntryResource
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	family := fetch("action=device_login")
	seen := map[string]int{}
	for _, e := range family {
		seen[e.Action]++
	}
	for _, action := range []string{
		store.AuditActionDeviceLoginApproved, store.AuditActionDeviceLoginDenied,
		store.AuditActionDeviceLoginExpired, store.AuditActionDeviceLoginDismissed,
	} {
		if seen[action] != 1 {
			t.Errorf("%s entries = %d, want 1 (seen %v)", action, seen[action], seen)
		}
	}
	if len(family) != 4 {
		t.Errorf("family size = %d, want 4", len(family))
	}
	if only := fetch("action=device_login.expired"); len(only) != 1 {
		t.Errorf("exact filter returned %d entries, want 1", len(only))
	}
	if byPath := fetch("path=" + store.DeviceLoginAuditPath(id)); len(byPath) != 2 {
		t.Errorf("per-request trail = %d entries, want expired and dismissed", len(byPath))
	}
}
