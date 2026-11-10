package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func createHook(t *testing.T, rt *Router, cookie *http.Cookie, body string) buildHookResource {
	t.Helper()
	rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/build-hooks", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	var res buildHookResource
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || res.Token == "" || res.Path == "" {
		t.Fatalf("create body = %s, %v", rec.Body.String(), err)
	}
	return res
}

func postHook(rt *Router, path, body string) *httptest.ResponseRecorder {
	return postHookFrom(rt, path, body, "203.0.113.9:4444")
}

func postHookFrom(rt *Router, path, body, remote string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.RemoteAddr = remote
	return serve(rt, req)
}

func TestBuildHookCreateValidation(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"ok", `{"name":"cms"}`, http.StatusCreated},
		{"with branch", `{"name":"cms-prod","branch":"main"}`, http.StatusCreated},
		{"empty name", `{"name":""}`, http.StatusBadRequest},
		{"bad name", `{"name":"a/b"}`, http.StatusBadRequest},
		{"branch with space", `{"name":"x","branch":"a b"}`, http.StatusBadRequest},
		{"bad json", `{`, http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt, _, cookie := newSafetyRouter(t, stubResolver{})
			rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/build-hooks", tc.body))
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestBuildHookDuplicateNameAndLimit(t *testing.T) {
	t.Setenv(envBuildHooksPerApp, "2")
	rt, _, cookie := newSafetyRouter(t, stubResolver{})
	createHook(t, rt, cookie, `{"name":"a"}`)
	if rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/build-hooks", `{"name":"a"}`)); rec.Code != http.StatusConflict {
		t.Errorf("duplicate = %d, want 409", rec.Code)
	}
	createHook(t, rt, cookie, `{"name":"b"}`)
	if rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/build-hooks", `{"name":"c"}`)); rec.Code != http.StatusConflict {
		t.Errorf("over limit = %d, want 409", rec.Code)
	}
}

func TestBuildHookSecretShownOnce(t *testing.T) {
	rt, db, cookie := newSafetyRouter(t, stubResolver{})
	res := createHook(t, rt, cookie, `{"name":"cms"}`)
	stored, err := db.GetBuildHook(context.Background(), res.ID)
	if err != nil || stored.TokenHash == "" || stored.TokenHash == res.Token {
		t.Fatalf("stored hash = %q, %v; want a hash distinct from the token", stored.TokenHash, err)
	}
	rec := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/build-hooks", ""))
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), res.Token) || strings.Contains(rec.Body.String(), stored.TokenHash) {
		t.Fatalf("list = %d leaked secret: %s", rec.Code, rec.Body.String())
	}
}

func TestBuildHookTriggerAuth(t *testing.T) {
	rt, db, cookie := newSafetyRouter(t, stubResolver{})
	res := createHook(t, rt, cookie, `{"name":"cms"}`)
	tests := []struct {
		name string
		path string
		want int
	}{
		{"valid", buildHookURLPrefix + res.ID + "/" + res.Token, http.StatusAccepted},
		{"wrong token", buildHookURLPrefix + res.ID + "/nope", http.StatusNotFound},
		{"unknown id", buildHookURLPrefix + "bh_missing/" + res.Token, http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := postHook(rt, tc.path, "").Code; got != tc.want {
				t.Fatalf("status = %d, want %d", got, tc.want)
			}
		})
	}
	h, err := db.GetBuildHook(context.Background(), res.ID)
	if err != nil || h.TriggerCount != 1 || h.LastTriggeredAt == nil {
		t.Fatalf("hook after one valid call = %+v, %v; want count 1 and a time", h, err)
	}
}

// The trigger is accepted and recorded even though the deploy cannot start (no git source): the half-success shows as the hook's last result.
func TestBuildHookTriggerRecordsFailedDeploy(t *testing.T) {
	rt, db, cookie := newSafetyRouter(t, stubResolver{})
	res := createHook(t, rt, cookie, `{"name":"cms"}`)
	if got := postHook(rt, res.Path, `{"title":"Post published"}`).Code; got != http.StatusAccepted {
		t.Fatalf("trigger = %d, want 202", got)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		h, err := db.GetBuildHook(context.Background(), res.ID)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(h.LastResult, "failed:") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("last result = %q, want a failed: result", h.LastResult)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestBuildHookRotateAndRevoke(t *testing.T) {
	rt, _, cookie := newSafetyRouter(t, stubResolver{})
	res := createHook(t, rt, cookie, `{"name":"cms"}`)

	rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/build-hooks/"+res.ID+"/rotate", ""))
	var rotated buildHookResource
	if err := json.Unmarshal(rec.Body.Bytes(), &rotated); rec.Code != http.StatusOK || err != nil || rotated.Token == "" || rotated.Token == res.Token {
		t.Fatalf("rotate = %d %s, %v", rec.Code, rec.Body.String(), err)
	}
	if got := postHook(rt, res.Path, "").Code; got != http.StatusNotFound {
		t.Errorf("old url after rotate = %d, want 404", got)
	}
	if got := postHook(rt, rotated.Path, "").Code; got != http.StatusAccepted {
		t.Errorf("new url after rotate = %d, want 202", got)
	}

	if rec := serve(rt, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/build-hooks/"+res.ID, "")); rec.Code != http.StatusNoContent {
		t.Fatalf("revoke = %d", rec.Code)
	}
	if got := postHook(rt, rotated.Path, "").Code; got != http.StatusNotFound {
		t.Errorf("url after revoke = %d, want 404", got)
	}
	if rec := serve(rt, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/build-hooks/"+res.ID, "")); rec.Code != http.StatusNotFound {
		t.Errorf("second revoke = %d, want 404", rec.Code)
	}
}

func TestBuildHookRateLimits(t *testing.T) {
	t.Run("per hook", func(t *testing.T) {
		t.Setenv(envBuildHookRate, "1")
		rt, _, cookie := newSafetyRouter(t, stubResolver{})
		res := createHook(t, rt, cookie, `{"name":"cms"}`)
		if got := postHook(rt, res.Path, "").Code; got != http.StatusAccepted {
			t.Fatalf("first = %d, want 202", got)
		}
		rec := postHook(rt, res.Path, "")
		if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
			t.Fatalf("second = %d retry-after %q, want 429 with Retry-After", rec.Code, rec.Header().Get("Retry-After"))
		}
	})
	t.Run("guessing from one address", func(t *testing.T) {
		t.Setenv(envBuildHookFailures, "2")
		rt, _, cookie := newSafetyRouter(t, stubResolver{})
		res := createHook(t, rt, cookie, `{"name":"cms"}`)
		for range 2 {
			postHook(rt, buildHookURLPrefix+res.ID+"/bad", "")
		}
		if got := postHook(rt, res.Path, "").Code; got != http.StatusTooManyRequests {
			t.Fatalf("after probes = %d, want 429", got)
		}
	})
}

func TestBuildHookPayloadAndArgs(t *testing.T) {
	t.Setenv(envBuildHookFieldMax, "5")
	tests := []struct {
		name      string
		target    string
		body      string
		wantTitle string
		wantBody  string
	}{
		{"query", "/x?title=hello&body=world", "", "hello", "world"},
		{"json overrides", "/x?title=a", `{"title":"bcd","body":"ef"}`, "bcd", "ef"},
		{"clipped", "/x", `{"title":"abcdefghij"}`, "abcde", ""},
		{"non json ignored", "/x?title=t", "not json", "t", ""},
		{"control chars stripped", "/x", "{\"title\":\"a\\u0000b\"}", "ab", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := readBuildHookPayload(httptest.NewRequest(http.MethodPost, tc.target, strings.NewReader(tc.body)))
			if p.Title != tc.wantTitle || p.Body != tc.wantBody {
				t.Fatalf("payload = %+v, want title %q body %q", p, tc.wantTitle, tc.wantBody)
			}
		})
	}
	base := map[string]string{"A": "1", "B": "2"}
	got := mergeBuildArgs(base, map[string]string{"B": "x", "C": "3"})
	if got["A"] != "1" || got["B"] != "x" || got["C"] != "3" || base["B"] != "2" {
		t.Errorf("merge = %v, base mutated = %v", got, base)
	}
}

func TestLoggablePath_RedactsBuildHookToken(t *testing.T) {
	if got := loggablePath(buildHookURLPrefix + "bh_1/s3cret"); strings.Contains(got, "s3cret") {
		t.Errorf("loggablePath leaked the token: %q", got)
	}
}

func TestBuildHookAllowlist(t *testing.T) {
	rt, db, cookie := newSafetyRouter(t, stubResolver{})
	res := createHook(t, rt, cookie, `{"name":"ci","allowed_cidrs":["198.51.100.0/24"]}`)
	if got := postHook(rt, res.Path, "").Code; got != http.StatusForbidden {
		t.Fatalf("outside allowlist = %d, want 403", got)
	}
	h, err := db.GetBuildHook(context.Background(), res.ID)
	if err != nil || h.TriggerCount != 0 || h.LastUsedAt == nil || !strings.HasPrefix(h.LastRefusal, "refused:") || h.LastResult != "" {
		t.Fatalf("hook after refused call = %+v, %v; want no run, a last-used time and a refusal kept apart from the result", h, err)
	}
	rec := serve(rt, authedRequest(t, cookie, http.MethodPatch, "/api/v1/apps/web/build-hooks/"+res.ID, `{"allowed_cidrs":["203.0.113.9","203.0.113.0/24","10.0.0.0/8"]}`))
	var upd buildHookResource
	if err := json.Unmarshal(rec.Body.Bytes(), &upd); rec.Code != http.StatusOK || err != nil {
		t.Fatalf("update = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Join(upd.AllowedCIDRs, ",") != "203.0.113.9/32,203.0.113.0/24,10.0.0.0/8" || upd.Token != "" {
		t.Fatalf("update allowlist = %v token %q, want canonical ranges and no secret", upd.AllowedCIDRs, upd.Token)
	}
	if got := postHook(rt, res.Path, "").Code; got != http.StatusAccepted {
		t.Fatalf("inside allowlist = %d, want 202", got)
	}
	if h, _ = db.GetBuildHook(context.Background(), res.ID); h.TriggerCount != 1 {
		t.Fatalf("trigger count = %d, want 1", h.TriggerCount)
	}
}

func TestBuildHookAllowlistValidation(t *testing.T) {
	t.Setenv("APP_BUILD_HOOK_MAX_CIDRS", "2")
	rt, db, cookie := newSafetyRouter(t, stubResolver{})
	for _, body := range []string{
		`{"name":"a","allowed_cidrs":["not-a-cidr"]}`,
		`{"name":"b","allowed_cidrs":["10.0.0.0/33"]}`,
		`{"name":"c","allowed_cidrs":["10.0.0.0/8","10.1.0.0/16","10.2.0.0/16"]}`,
		`{"name":"d","allowed_cidrs":["10.1.2.3/8"]}`,
		`{"name":"e","allowed_cidrs":["0.0.0.0/0"]}`,
	} {
		if rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/build-hooks", body)); rec.Code != http.StatusBadRequest {
			t.Errorf("create %s = %d, want 400", body, rec.Code)
		}
	}
	if rec := serve(rt, authedRequest(t, cookie, http.MethodPatch, "/api/v1/apps/web/build-hooks/bh_missing", `{"allow_any":true}`)); rec.Code != http.StatusNotFound {
		t.Errorf("update unknown = %d, want 404", rec.Code)
	}
	res := createHook(t, rt, cookie, `{"name":"ci","allowed_cidrs":["198.51.100.0/24"]}`)
	path := "/api/v1/apps/web/build-hooks/" + res.ID
	for _, body := range []string{`{}`, `{"allowed_cidrs":[]}`, `{"allowed_cidrs":null}`, `{"cidrs":["10.0.0.0/8"]}`, `{"allowed_cidrs":["::/0"]}`, `{"allowed_cidrs":["10.0.0.0/8"],"allow_any":true}`} {
		if rec := serve(rt, authedRequest(t, cookie, http.MethodPatch, path, body)); rec.Code != http.StatusBadRequest {
			t.Errorf("update %s = %d, want 400", body, rec.Code)
		}
	}
	if h, _ := db.GetBuildHook(context.Background(), res.ID); len(h.AllowedCIDRs) != 1 {
		t.Fatalf("rejected updates changed the allowlist: %v", h.AllowedCIDRs)
	}
	if rec := serve(rt, authedRequest(t, cookie, http.MethodPatch, path, `{"allow_any":true}`)); rec.Code != http.StatusOK {
		t.Fatalf("allow_any = %d %s", rec.Code, rec.Body.String())
	}
	if h, _ := db.GetBuildHook(context.Background(), res.ID); len(h.AllowedCIDRs) != 0 {
		t.Fatalf("allow_any left %v", h.AllowedCIDRs)
	}
}

// Refused sources spend their own bucket: they are throttled, and an allowed caller still gets through.
func TestBuildHookRefusedDoNotDrainAllowed(t *testing.T) {
	t.Setenv(envBuildHookRate, "1")
	rt, db, cookie := newSafetyRouter(t, stubResolver{})
	res := createHook(t, rt, cookie, `{"name":"ci","allowed_cidrs":["198.51.100.0/24"]}`)
	if got := postHook(rt, res.Path, "").Code; got != http.StatusForbidden {
		t.Fatalf("first refused = %d, want 403", got)
	}
	if got := postHook(rt, res.Path, "").Code; got != http.StatusTooManyRequests {
		t.Fatalf("second refused = %d, want 429", got)
	}
	if got := postHookFrom(rt, res.Path, "", "198.51.100.5:4444").Code; got != http.StatusAccepted {
		t.Fatalf("allowed caller after refusals = %d, want 202", got)
	}
	if got := postHookFrom(rt, res.Path, "", "198.51.100.5:4444").Code; got != http.StatusTooManyRequests {
		t.Fatalf("allowed caller over limit = %d, want 429", got)
	}
	if h, _ := db.GetBuildHook(context.Background(), res.ID); h.TriggerCount != 1 || h.LastRefusal != "refused: source address not allowed" {
		t.Fatalf("hook = %+v, want one run and only the first refusal recorded", h)
	}
}
