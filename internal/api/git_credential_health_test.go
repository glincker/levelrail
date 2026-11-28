package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/attention"
	"github.com/glincker/levelrail/internal/eventhook"
	"github.com/glincker/levelrail/internal/gitlabapp"
	"github.com/glincker/levelrail/internal/gitprovider"
	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
)

// credFakeGitLab adds the token info and self rotation calls to the shared GitLab fake.
type credFakeGitLab struct {
	*fakeGitLabAppClient
	info      gitlabapp.TokenInfo
	rotateTok string
	rotateExp string
	rotateErr error
	rotated   int
}

func (f *credFakeGitLab) TokenInfo(context.Context, string, string) (gitlabapp.TokenInfo, error) {
	return f.info, nil
}

func (f *credFakeGitLab) RotateSelf(context.Context, string, string, time.Time) (string, string, error) {
	f.rotated++
	return f.rotateTok, f.rotateExp, f.rotateErr
}

func newCredRouter(t *testing.T, env map[string]string, gl *credFakeGitLab) (*Router, *store.DB, *fakeGitConnSecrets, *captureWebhooks) {
	t.Helper()
	secrets := newFakeGitConnSecrets()
	rt, db := newGitConnTestRouter(t, secrets)
	rt.gitlabAppClient = gl
	rt.runtimeSettings = runtimesettings.New(db, func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	hooks := &captureWebhooks{}
	rt.eventWebhooks = hooks
	return rt, db, secrets, hooks
}

func gitEvents(h *captureWebhooks, typ string) []eventhook.Event {
	var out []eventhook.Event
	for _, e := range h.events {
		if e.Type == typ {
			out = append(out, e)
		}
	}
	return out
}

func credItems(t *testing.T, rt *Router) []attention.Item {
	t.Helper()
	return rt.gitCredentialAttentionItems(httptest.NewRequest(http.MethodGet, "/", nil), []string{AbilityRoot}, time.Now())
}

func dueOAuthConn(t *testing.T, db *store.DB, secrets GitConnSecrets) store.GitConnection {
	t.Helper()
	c := seedGitConn(t, db, secrets, gitProviderGitLab, "gitlab.example.com", "ann", true)
	mustSetGitConnSecret(t, secrets, c.SecretsKey, gitConnTokenExpiresKey, strconv.FormatInt(time.Now().Add(2*time.Minute).Unix(), 10))
	return c
}

func staticGitLabConn(t *testing.T, db *store.DB, secrets GitConnSecrets, account string) store.GitConnection {
	t.Helper()
	c := seedGitConn(t, db, secrets, gitProviderGitLab, "gitlab.example.com", account, true)
	mustSetGitConnSecret(t, secrets, c.SecretsKey, gitConnRefreshTokenKey, "")
	return c
}

func TestGitCredentialKindAndFields(t *testing.T) {
	rt, db, secrets, _ := newCredRouter(t, nil, &credFakeGitLab{fakeGitLabAppClient: &fakeGitLabAppClient{}})
	oauth := seedGitConn(t, db, secrets, gitProviderGitLab, "a.example.com", "ann", true)
	static := staticGitLabConn(t, db, secrets, "bob")
	if got := rt.gitConnectionResourceFor(t.Context(), oauth); got.CredentialKind != gitCredOAuth || !got.AutoRenew || got.CanRotate || got.ExpiresAt == "" {
		t.Fatalf("oauth resource = %+v", got)
	}
	if got := rt.gitConnectionResourceFor(t.Context(), static); got.CredentialKind != gitCredStatic || got.AutoRenew || !got.CanRotate {
		t.Fatalf("static resource = %+v", got)
	}
}

func TestGitOAuthRefreshSuccessIsSilent(t *testing.T) {
	fake := &credFakeGitLab{fakeGitLabAppClient: &fakeGitLabAppClient{refreshTokens: gitlabapp.Tokens{AccessToken: "at-new", RefreshToken: "rt-new", ExpiresAt: time.Now().Add(time.Hour)}}}
	rt, db, secrets, hooks := newCredRouter(t, nil, fake)
	c := dueOAuthConn(t, db, secrets)
	rt.SweepGitOAuth(t.Context(), time.Now())
	if got := fake.refreshCount.Load(); got != 1 {
		t.Fatalf("refresh calls = %d, want 1", got)
	}
	if v, _ := secrets.Resolve(t.Context(), c.SecretsKey, gitConnAccessTokenKey); v != "at-new" {
		t.Fatalf("access token not replaced")
	}
	if got := rt.gitConnectionResourceFor(t.Context(), c); got.LastRefreshAt == "" || got.LastRefreshError != "" {
		t.Fatalf("resource = %+v", got)
	}
	if len(hooks.events) != 0 || len(credItems(t, rt)) != 0 {
		t.Fatalf("a successful refresh must be silent: events %v items %v", hooks.events, credItems(t, rt))
	}
}

func TestGitOAuthRefreshFailureRaisesOneDedupedAlertAndClears(t *testing.T) {
	fake := &credFakeGitLab{fakeGitLabAppClient: &fakeGitLabAppClient{refreshErr: errors.New("dial tcp: host unreachable")}}
	rt, db, secrets, hooks := newCredRouter(t, map[string]string{"APP_GIT_OAUTH_REFRESH_FAILURE_GRACE": "5m"}, fake)
	c := dueOAuthConn(t, db, secrets)
	now := time.Now()

	rt.SweepGitOAuth(t.Context(), now)
	if len(hooks.events) != 0 || len(credItems(t, rt)) != 0 {
		t.Fatal("a failure inside the grace must not alert")
	}
	mustSetGitConnSecret(t, secrets, c.SecretsKey, gitConnRefreshFailSinceKey, strconv.FormatInt(now.Add(-10*time.Minute).Unix(), 10))
	rt.SweepGitOAuth(t.Context(), now)
	rt.SweepGitOAuth(t.Context(), now)
	if got := len(gitEvents(hooks, eventGitReauthNeeded)); got != 1 {
		t.Fatalf("reauth events = %d, want exactly 1", got)
	}
	items := credItems(t, rt)
	if len(items) != 1 || items[0].Kind != attention.KindGitConnectionReauth || !strings.Contains(items[0].Link, "reauthorize=1") {
		t.Fatalf("items = %+v", items)
	}

	fake.refreshErr = nil
	fake.refreshTokens = gitlabapp.Tokens{AccessToken: "at-ok", RefreshToken: "rt-ok", ExpiresAt: now.Add(time.Hour)}
	rt.SweepGitOAuth(t.Context(), now)
	if len(credItems(t, rt)) != 0 {
		t.Fatal("the alert must clear after a successful refresh")
	}
	sent, _ := rt.tokenStateLoad(t.Context(), tokenStateNotices)
	if _, left := sent[gitReauthStatePrefix+c.ID]; left {
		t.Fatal("the dedup mark must clear with the alert")
	}
}

func TestGitOAuthRevokedGrantAlertsAtOnceAndIsNotRetried(t *testing.T) {
	bad := &gitprovider.APIError{Prefix: "gitlabapp", API: "gitlab", StatusCode: http.StatusBadRequest, Body: `{"error":"invalid_grant"}`}
	fake := &credFakeGitLab{fakeGitLabAppClient: &fakeGitLabAppClient{refreshErr: bad}}
	rt, db, secrets, hooks := newCredRouter(t, nil, fake)
	dueOAuthConn(t, db, secrets)
	rt.SweepGitOAuth(t.Context(), time.Now())
	rt.SweepGitOAuth(t.Context(), time.Now())
	if len(gitEvents(hooks, eventGitReauthNeeded)) != 1 || len(credItems(t, rt)) != 1 {
		t.Fatalf("events %v items %v", hooks.events, credItems(t, rt))
	}
	if got := fake.refreshCount.Load(); got != 1 {
		t.Fatalf("refresh calls = %d, want 1: a connection that needs reauthorization is skipped", got)
	}
}

func TestGitOAuthRefreshOffSwitch(t *testing.T) {
	fake := &credFakeGitLab{fakeGitLabAppClient: &fakeGitLabAppClient{refreshTokens: gitlabapp.Tokens{AccessToken: "x", RefreshToken: "y", ExpiresAt: time.Now().Add(time.Hour)}}}
	rt, db, secrets, _ := newCredRouter(t, map[string]string{"APP_GIT_OAUTH_REFRESH_ENABLED": "false"}, fake)
	c := dueOAuthConn(t, db, secrets)
	rt.SweepGitOAuth(t.Context(), time.Now())
	if fake.refreshCount.Load() != 0 {
		t.Fatal("the off switch must stop the proactive refresh")
	}
	if got := rt.gitConnectionResourceFor(t.Context(), c); got.AutoRenew {
		t.Fatal("auto_renew must be false while the switch is off")
	}
	if res := rt.doctorCheckGitOAuthRefresh(t.Context()); res.Status != doctorStatusWarn {
		t.Fatalf("doctor = %+v, want a warning while off", res)
	}
}

func TestGitStaticTokenReminderWindows(t *testing.T) {
	fake := &credFakeGitLab{fakeGitLabAppClient: &fakeGitLabAppClient{}}
	rt, db, secrets, hooks := newCredRouter(t, nil, fake)
	staticGitLabConn(t, db, secrets, "ann")
	now := time.Now()

	fake.info = gitlabapp.TokenInfo{ExpiresAt: now.Add(30 * 24 * time.Hour).Format(time.RFC3339)}
	if err := rt.SweepGitStaticTokens(t.Context(), now); err != nil || len(hooks.events) != 0 || len(credItems(t, rt)) != 0 {
		t.Fatalf("outside every window: err %v events %v", err, hooks.events)
	}
	fake.info = gitlabapp.TokenInfo{ExpiresAt: now.Add(5 * 24 * time.Hour).Format(time.RFC3339)}
	for range 2 {
		if err := rt.SweepGitStaticTokens(t.Context(), now); err != nil {
			t.Fatal(err)
		}
	}
	ev := gitEvents(hooks, eventGitExpiring)
	if len(ev) != 1 || ev[0].Data["window_days"] != 7 {
		t.Fatalf("events = %+v, want one for the 7 day window", ev)
	}
	items := credItems(t, rt)
	if len(items) != 1 || items[0].Kind != attention.KindGitTokenExpiring {
		t.Fatalf("items = %+v", items)
	}
	fake.info = gitlabapp.TokenInfo{ExpiresAt: now.Add(12 * time.Hour).Format(time.RFC3339)}
	if err := rt.SweepGitStaticTokens(t.Context(), now); err != nil || len(gitEvents(hooks, eventGitExpiring)) != 2 {
		t.Fatalf("the narrower window must fire once more, events %v", hooks.events)
	}
}

func postRotate(t *testing.T, rt *Router, db *store.DB, id string) *httptest.ResponseRecorder {
	t.Helper()
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/git-connections/"+id+"/rotate", ""))
	return rec
}

func TestGitLabRotateStoresNewTokenAndReturnsMetadataOnly(t *testing.T) {
	fake := &credFakeGitLab{fakeGitLabAppClient: &fakeGitLabAppClient{}, rotateTok: "glpat-new-secret", rotateExp: "2027-01-31"}
	rt, db, secrets, _ := newCredRouter(t, nil, fake)
	c := staticGitLabConn(t, db, secrets, "ann")
	rec := postRotate(t, rt, db, c.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "glpat-new-secret") || strings.Contains(rec.Body.String(), "at-ann") {
		t.Fatal("the response must not carry any token")
	}
	var out gitRotateResource
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || !out.Rotated || out.ExpiresAt != "2027-01-31" {
		t.Fatalf("response = %+v err %v", out, err)
	}
	if v, _ := secrets.Resolve(t.Context(), c.SecretsKey, gitConnAccessTokenKey); v != "glpat-new-secret" {
		t.Fatal("the new token was not stored")
	}
	if got := rt.gitConnectionResourceFor(t.Context(), c); !strings.HasPrefix(got.ExpiresAt, "2027-01-31") {
		t.Fatalf("expires_at = %q", got.ExpiresAt)
	}
}

func TestGitRotateRefusesOAuthWith409(t *testing.T) {
	fake := &credFakeGitLab{fakeGitLabAppClient: &fakeGitLabAppClient{}}
	rt, db, secrets, _ := newCredRouter(t, nil, fake)
	c := seedGitConn(t, db, secrets, gitProviderGitLab, "gitlab.example.com", "ann", true)
	rec := postRotate(t, rt, db, c.ID)
	if rec.Code != http.StatusConflict || fake.rotated != 0 {
		t.Fatalf("status = %d rotated %d body %s", rec.Code, fake.rotated, rec.Body.String())
	}
	if rec := postRotate(t, rt, db, "gc_missing"); rec.Code != http.StatusNotFound {
		t.Fatalf("missing connection status = %d", rec.Code)
	}
}
