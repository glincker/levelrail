package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type addressedMail struct {
	mu   sync.Mutex
	sent []struct{ to, body string }
}

func (m *addressedMail) Send(_ context.Context, to, _, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, struct{ to, body string }{to, body})
	return nil
}

func (m *addressedMail) last(to string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.sent) - 1; i >= 0; i-- {
		if m.sent[i].to == to {
			return m.sent[i].body
		}
	}
	return ""
}

func (m *addressedMail) count(to string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, s := range m.sent {
		if s.to == to {
			n++
		}
	}
	return n
}

func accountOtherUser(t *testing.T, h *codeHarness) *http.Cookie {
	t.Helper()
	other := store.User{ID: "user_other", Email: "other@example.com", DisplayName: "Other", Abilities: []string{AbilityRead}, CreatedAt: time.Now()}
	if err := h.db.CreateUser(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	return sessionCookieForTest(t, h.rt, other.ID)
}

func seedOwnedToken(t *testing.T, h *codeHarness, id, owner string, abilities []string) string {
	t.Helper()
	plain := "acct-secret-" + id
	err := h.db.SaveAPIToken(context.Background(), store.APIToken{ID: id, Name: id, TokenHash: hashToken(plain), Abilities: abilities, OwnerUserID: owner})
	if err != nil {
		t.Fatal(err)
	}
	return plain
}

func fetchActivity(t *testing.T, h *codeHarness, query string, cookie *http.Cookie) myActivityResponse {
	t.Helper()
	rec := h.send(http.MethodGet, "/api/v1/auth/my-activity"+query, "", "", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("my-activity status = %d: %s", rec.Code, rec.Body.String())
	}
	var out myActivityResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMyActivity_OnlyCallersOwnRows(t *testing.T) {
	h := newCodeHarness(t, false)
	otherCookie := accountOtherUser(t, h)
	own := h.userSession()

	if rec := h.send(http.MethodPatch, "/api/v1/auth/profile", `{"display_name":"Mine"}`, "", own); rec.Code != http.StatusOK {
		t.Fatalf("profile status = %d", rec.Code)
	}
	if rec := h.send(http.MethodPatch, "/api/v1/auth/profile", `{"display_name":"Theirs"}`, "", otherCookie); rec.Code != http.StatusOK {
		t.Fatalf("other profile status = %d", rec.Code)
	}

	mine := fetchActivity(t, h, "", own)
	if len(mine.Entries) != 1 || mine.Entries[0].Event != accountEventProfileUpdated || mine.Entries[0].Kind != activityKindAccount {
		t.Fatalf("own feed = %+v, want exactly one profile_updated row", mine.Entries)
	}
	// A caller-supplied user id must change nothing: the account comes from the credential.
	spoof := fetchActivity(t, h, "?user_id=user_other&owner=user_other", own)
	if len(spoof.Entries) != 1 {
		t.Fatalf("spoofed query returned %d rows, want the caller's own 1", len(spoof.Entries))
	}
	theirs := fetchActivity(t, h, "", otherCookie)
	if len(theirs.Entries) != 1 {
		t.Fatalf("other feed = %d rows, want 1", len(theirs.Entries))
	}
}

func TestMyActivity_FiltersAndPaging(t *testing.T) {
	h := newCodeHarness(t, false)
	own := h.userSession()
	for i := 0; i < 3; i++ {
		body := `{"timezone":"UTC","language":"","theme":"dark","landing_page":"","gravatar":false}`
		if rec := h.send(http.MethodPut, "/api/v1/auth/preferences", body, "", own); rec.Code != http.StatusOK {
			t.Fatalf("prefs status = %d: %s", rec.Code, rec.Body.String())
		}
		time.Sleep(2 * time.Millisecond)
	}
	page1 := fetchActivity(t, h, "?limit=2", own)
	if len(page1.Entries) != 2 || page1.NextCursor == "" {
		t.Fatalf("page 1 = %d rows cursor %q, want 2 rows and a cursor", len(page1.Entries), page1.NextCursor)
	}
	page2 := fetchActivity(t, h, "?limit=2&cursor="+page1.NextCursor, own)
	if len(page2.Entries) != 1 || page2.NextCursor != "" {
		t.Fatalf("page 2 = %d rows cursor %q, want the last row", len(page2.Entries), page2.NextCursor)
	}
	if got := fetchActivity(t, h, "?kind=sign_in", own); len(got.Entries) != 0 {
		t.Fatalf("sign_in filter returned %d rows, want 0", len(got.Entries))
	}
	if got := fetchActivity(t, h, "?kind=account", own); len(got.Entries) != 3 {
		t.Fatalf("account filter returned %d rows, want 3", len(got.Entries))
	}
	for _, q := range []string{"?kind=bogus", "?limit=0", "?limit=101", "?cursor=nope"} {
		if rec := h.send(http.MethodGet, "/api/v1/auth/my-activity"+q, "", "", own); rec.Code != http.StatusBadRequest {
			t.Errorf("%s status = %d, want 400", q, rec.Code)
		}
	}
}

func TestMyActivity_TokenAccess(t *testing.T) {
	h := newCodeHarness(t, false)
	readOnly := seedOwnedToken(t, h, "ro", h.user.ID, []string{AbilityRead})
	power := store.User{ID: "user_power", Email: "power@example.com", DisplayName: "Power", Abilities: []string{AbilityRead, AbilityReadSensitive}, CreatedAt: time.Now()}
	if err := h.db.CreateUser(context.Background(), power); err != nil {
		t.Fatal(err)
	}
	sensitive := seedOwnedToken(t, h, "sens", power.ID, []string{AbilityRead, AbilityReadSensitive})
	orphan := seedOwnedToken(t, h, "orphan", "", []string{AbilityRead, AbilityReadSensitive})
	tests := []struct {
		name  string
		token string
		want  int
	}{
		{"read-only token lacks read:sensitive", readOnly, http.StatusForbidden},
		{"token with read:sensitive and an owner", sensitive, http.StatusOK},
		{"token without an owning user", orphan, http.StatusForbidden},
		{"anonymous", "", http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := h.send(http.MethodGet, "/api/v1/auth/my-activity", "", "")
			if tt.token != "" {
				req := bearerRequest(http.MethodGet, "/api/v1/auth/my-activity", "", tt.token)
				rec = serve(h.rt, req)
			}
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestPreferences(t *testing.T) {
	h := newCodeHarness(t, false)
	own := h.userSession()
	otherCookie := accountOtherUser(t, h)

	if rec := h.send(http.MethodGet, "/api/v1/auth/preferences", "", "", own); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"gravatar":false`) {
		t.Fatalf("default preferences = %d %s, want gravatar off", rec.Code, rec.Body.String())
	}
	tests := []struct {
		name string
		body string
		want int
	}{
		{"valid", `{"timezone":"Europe/Berlin","language":"pt-BR","theme":"dark","landing_page":"/apps","gravatar":true}`, http.StatusOK},
		{"empty means default", `{}`, http.StatusOK},
		{"bad timezone", `{"timezone":"Mars/Base"}`, http.StatusBadRequest},
		{"bad language", `{"language":"english please"}`, http.StatusBadRequest},
		{"bad theme", `{"theme":"neon"}`, http.StatusBadRequest},
		{"landing page with scheme", `{"landing_page":"https://evil.example"}`, http.StatusBadRequest},
		{"landing page protocol relative", `{"landing_page":"//evil.example"}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := h.send(http.MethodPut, "/api/v1/auth/preferences", tt.body, "", own); rec.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
	h.send(http.MethodPut, "/api/v1/auth/preferences", `{"timezone":"Asia/Tokyo","theme":"light"}`, "", own)
	if rec := h.send(http.MethodGet, "/api/v1/auth/preferences", "", "", otherCookie); strings.Contains(rec.Body.String(), "Tokyo") {
		t.Fatalf("another user's preferences leaked: %s", rec.Body.String())
	}
	if rec := h.send(http.MethodGet, "/api/v1/auth/preferences", "", "", own); !strings.Contains(rec.Body.String(), "Asia/Tokyo") {
		t.Fatalf("saved preferences not returned: %s", rec.Body.String())
	}
	if rec := h.send(http.MethodPut, "/api/v1/auth/preferences", `{}`, "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous PUT status = %d, want 401", rec.Code)
	}
	ro := seedOwnedToken(t, h, "pro", h.user.ID, []string{AbilityRead})
	if rec := serve(h.rt, bearerRequest(http.MethodPut, "/api/v1/auth/preferences", `{}`, ro)); rec.Code != http.StatusForbidden {
		t.Fatalf("read-only token PUT status = %d, want 403", rec.Code)
	}
}

func TestUnlinkProvider_LastSignInMethodGuard(t *testing.T) {
	h := newCodeHarness(t, false)
	ctx := context.Background()
	own := h.userSession()
	link := func(userID, provider string) {
		t.Helper()
		err := h.db.SaveOAuthIdentity(ctx, store.OAuthIdentity{ID: "oi_" + userID + provider, UserID: userID, Provider: provider, ProviderUserID: "p-" + userID + provider, CreatedAt: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
	}
	link(h.user.ID, store.OAuthProviderGitHub)
	if rec := h.send(http.MethodDelete, "/api/v1/auth/link/github", "", "", own); rec.Code != http.StatusNoContent {
		t.Fatalf("unlink with a password status = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := h.send(http.MethodDelete, "/api/v1/auth/link/github", "", "", own); rec.Code != http.StatusNotFound {
		t.Fatalf("second unlink status = %d, want 404", rec.Code)
	}

	oauthOnly := store.User{ID: "user_sso", Email: "sso@example.com", DisplayName: "SSO", Abilities: []string{AbilityRead}, CreatedAt: time.Now()}
	if err := h.db.CreateUser(ctx, oauthOnly); err != nil {
		t.Fatal(err)
	}
	link(oauthOnly.ID, store.OAuthProviderGitHub)
	link(oauthOnly.ID, store.OAuthProviderGoogle)
	ssoCookie := sessionCookieForTest(t, h.rt, oauthOnly.ID)
	if rec := h.send(http.MethodDelete, "/api/v1/auth/link/github", "", "", ssoCookie); rec.Code != http.StatusNoContent {
		t.Fatalf("unlink one of two status = %d: %s", rec.Code, rec.Body.String())
	}
	rec := h.send(http.MethodDelete, "/api/v1/auth/link/google", "", "", ssoCookie)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "last_sign_in_method") {
		t.Fatalf("unlink the only way in = %d %s, want 409 last_sign_in_method", rec.Code, rec.Body.String())
	}
	ids, _ := h.db.ListOAuthIdentitiesForUser(ctx, oauthOnly.ID)
	if len(ids) != 1 {
		t.Fatalf("identities left = %d, want the guarded one to stay", len(ids))
	}
	if rec := h.send(http.MethodDelete, "/api/v1/auth/link/not-a-provider", "", "", own); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown provider status = %d, want 404", rec.Code)
	}
}

func TestDeleteAccount(t *testing.T) {
	h := newCodeHarness(t, false)
	own := h.userSession()
	admin := adminSessionForTest(t, h)

	rec := h.send(http.MethodGet, "/api/v1/auth/delete-account/check", "", "", admin)
	var adminCheck deleteCheckResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &adminCheck)
	if adminCheck.CanDelete || len(adminCheck.Blockers) == 0 || adminCheck.Blockers[0].Code != deleteBlockLastRoot {
		t.Fatalf("admin check = %+v, want the last root blocker", adminCheck)
	}

	tests := []struct {
		name string
		body string
		want int
		code string
	}{
		{"wrong confirmation", `{"confirm":"nope@example.com","password":"` + codeTestPassword + `"}`, http.StatusBadRequest, "confirmation_mismatch"},
		{"no proof", `{"confirm":"` + codeTestEmail + `"}`, http.StatusForbidden, errCodeStepUpRequired},
		{"wrong password", `{"confirm":"` + codeTestEmail + `","password":"wrong-password-here"}`, http.StatusForbidden, "step_up_invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := h.send(http.MethodDelete, "/api/v1/auth/account", tt.body, "", own)
			if rec.Code != tt.want || !strings.Contains(rec.Body.String(), tt.code) {
				t.Fatalf("status = %d %s, want %d %s", rec.Code, rec.Body.String(), tt.want, tt.code)
			}
		})
	}
	if _, err := h.db.GetUserByID(context.Background(), h.user.ID); err != nil {
		t.Fatalf("user vanished after refused deletions: %v", err)
	}
	if rec := h.send(http.MethodDelete, "/api/v1/auth/account", `{"confirm":"`+codeTestEmail+`","password":"`+codeTestPassword+`"}`, "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous delete status = %d, want 401", rec.Code)
	}
	rec = h.send(http.MethodDelete, "/api/v1/auth/account", `{"confirm":"`+strings.ToUpper(codeTestEmail)+`","password":"`+codeTestPassword+`"}`, "", own)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := h.db.GetUserByID(context.Background(), h.user.ID); err == nil {
		t.Fatal("user still exists after deletion")
	}
	if rec := h.send(http.MethodGet, "/api/v1/auth/my-access", "", "", own); rec.Code != http.StatusUnauthorized {
		t.Fatalf("deleted user's session status = %d, want 401", rec.Code)
	}
}

func TestDeleteAccount_LastRootRefusedEvenWithProof(t *testing.T) {
	h := newCodeHarness(t, false)
	admin, err := h.db.GetUserByEmail(context.Background(), testAdminUsername)
	if err != nil {
		t.Fatal(err)
	}
	if admin.PasswordHash == nil {
		t.Skip("bootstrap admin has no password to prove with")
	}
	rec := h.send(http.MethodDelete, "/api/v1/auth/account", `{"confirm":"`+admin.Email+`","password":"x"}`, "", sessionCookieForTest(t, h.rt, admin.ID))
	if rec.Code == http.StatusNoContent {
		t.Fatal("the last root account was deleted")
	}
}

func TestExport_BundleIsOwnedAndSecretFree(t *testing.T) {
	h := newCodeHarness(t, false)
	own := h.userSession()
	otherCookie := accountOtherUser(t, h)
	plain := seedOwnedToken(t, h, "exp-tok", h.user.ID, []string{AbilityRead})

	rec := h.send(http.MethodPost, "/api/v1/auth/export", "", "", own)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("start status = %d: %s", rec.Code, rec.Body.String())
	}
	var job accountExportResource
	_ = json.Unmarshal(rec.Body.Bytes(), &job)

	deadline := time.Now().Add(10 * time.Second)
	for job.Status == accountExportPending && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		rec = h.send(http.MethodGet, "/api/v1/auth/export/"+job.ID, "", "", own)
		_ = json.Unmarshal(rec.Body.Bytes(), &job)
	}
	if job.Status != accountExportReady {
		t.Fatalf("export status = %q, want ready", job.Status)
	}
	if rec := h.send(http.MethodGet, "/api/v1/auth/export/"+job.ID, "", "", otherCookie); rec.Code != http.StatusNotFound {
		t.Fatalf("another user's status read = %d, want 404", rec.Code)
	}
	if rec := h.send(http.MethodGet, "/api/v1/auth/export/"+job.ID+"/download", "", "", otherCookie); rec.Code != http.StatusNotFound {
		t.Fatalf("another user's download = %d, want 404", rec.Code)
	}
	dl := h.send(http.MethodGet, "/api/v1/auth/export/"+job.ID+"/download", "", "", own)
	if dl.Code != http.StatusOK || dl.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("download = %d %s", dl.Code, dl.Header().Get("Content-Type"))
	}
	zr, err := zip.NewReader(bytes.NewReader(dl.Body.Bytes()), int64(dl.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		_ = r.Close()
		for _, secret := range []string{plain, hashToken(plain), "password_hash", "PasswordHash"} {
			if strings.Contains(buf.String(), secret) {
				t.Fatalf("%s leaks %q", f.Name, secret)
			}
		}
		if f.Name == "profile.json" && strings.Contains(buf.String(), "other@example.com") {
			t.Fatal("profile.json contains another user")
		}
	}
	for _, want := range []string{"profile.json", "sessions.json", "tokens.json", "activity.json", "manifest.json"} {
		if !names[want] {
			t.Errorf("bundle is missing %s (has %v)", want, names)
		}
	}
}

func TestEmailChange(t *testing.T) {
	h := newCodeHarness(t, false)
	mail := &addressedMail{}
	h.rt.emailSender = mail
	own := h.userSession()
	otherCookie := accountOtherUser(t, h)
	const next = "new@example.com"

	if rec := h.send(http.MethodPost, "/api/v1/auth/email-change", `{"new_email":"not-an-email","password":"`+codeTestPassword+`"}`, "", own); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad address status = %d, want 400", rec.Code)
	}
	if rec := h.send(http.MethodPost, "/api/v1/auth/email-change", `{"new_email":"`+next+`","password":"wrong-password-here"}`, "", own); rec.Code != http.StatusForbidden {
		t.Fatalf("wrong password status = %d, want 403", rec.Code)
	}
	if mail.count(next) != 0 {
		t.Fatal("mail went out before the password was proven")
	}
	if rec := h.send(http.MethodPost, "/api/v1/auth/email-change", `{"new_email":"`+next+`","password":"`+codeTestPassword+`"}`, "", own); rec.Code != http.StatusAccepted {
		t.Fatalf("request status = %d", rec.Code)
	}
	body := mail.last(next)
	idx := strings.Index(body, "confirm_email=")
	if idx < 0 {
		t.Fatalf("mail has no confirm link: %q", body)
	}
	token := strings.Fields(body[idx+len("confirm_email="):])[0]

	if rec := h.send(http.MethodPost, "/api/v1/auth/email-change/confirm", `{"token":"`+token+`"}`, "", otherCookie); rec.Code != http.StatusBadRequest {
		t.Fatalf("another account confirming status = %d, want 400", rec.Code)
	}
	if rec := h.send(http.MethodPost, "/api/v1/auth/email-change/confirm", `{"token":"`+token+`"}`, "", own); rec.Code != http.StatusOK {
		t.Fatalf("confirm status = %d: %s", rec.Code, rec.Body.String())
	}
	u, err := h.db.GetUserByID(context.Background(), h.user.ID)
	if err != nil || u.Email != next {
		t.Fatalf("email = %q (%v), want %q", u.Email, err, next)
	}
	if mail.count(codeTestEmail) == 0 {
		t.Error("the old address got no change notice")
	}
	if rec := h.send(http.MethodPost, "/api/v1/auth/email-change/confirm", `{"token":"`+token+`"}`, "", own); rec.Code != http.StatusBadRequest {
		t.Fatalf("token reuse status = %d, want 400", rec.Code)
	}
	before := mail.count("other@example.com")
	h.send(http.MethodPost, "/api/v1/auth/email-change", `{"new_email":"other@example.com","password":"`+codeTestPassword+`"}`, "", own)
	if mail.count("other@example.com") != before {
		t.Fatal("mail was sent to an address another account holds")
	}
}

func TestEmailChange_NeedsMailer(t *testing.T) {
	h := newCodeHarness(t, false)
	h.rt.emailSender = nil
	rec := h.send(http.MethodPost, "/api/v1/auth/email-change", `{"new_email":"a@example.com","password":"`+codeTestPassword+`"}`, "", h.userSession())
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "email_not_configured") {
		t.Fatalf("status = %d %s, want 409 email_not_configured", rec.Code, rec.Body.String())
	}
	rec = h.send(http.MethodGet, "/api/v1/auth/email-change", "", "", h.userSession())
	if !strings.Contains(rec.Body.String(), `"available":false`) {
		t.Fatalf("status body = %s, want available false", rec.Body.String())
	}
}

func TestMatchAccountEventRoute(t *testing.T) {
	tests := []struct {
		method, path, want string
		ok                 bool
	}{
		{http.MethodPut, "/api/v1/auth/password", accountEventPasswordChanged, true},
		{http.MethodDelete, "/api/v1/auth/passkeys/abc", accountEventPasskeyRemoved, true},
		{http.MethodDelete, "/api/v1/auth/passkeys/", "", false},
		{http.MethodGet, "/api/v1/auth/password", "", false},
		{http.MethodPost, "/api/v1/auth/tokens", accountEventTokenCreated, true},
	}
	for _, tt := range tests {
		got, ok := matchAccountEventRoute(tt.method, tt.path)
		if got != tt.want || ok != tt.ok {
			t.Errorf("%s %s = %q %v, want %q %v", tt.method, tt.path, got, ok, tt.want, tt.ok)
		}
	}
}

func TestPreferencesProblem(t *testing.T) {
	tests := []struct {
		name string
		in   preferencesResource
		bad  bool
	}{
		{"zero value", preferencesResource{}, false},
		{"utc", preferencesResource{Timezone: "UTC"}, false},
		{"nested zone", preferencesResource{Timezone: "America/Argentina/Buenos_Aires"}, false},
		{"unknown zone", preferencesResource{Timezone: "Nowhere/City"}, true},
		{"long zone", preferencesResource{Timezone: strings.Repeat("a", 65)}, true},
		{"bare language", preferencesResource{Language: "de"}, false},
		{"regional language", preferencesResource{Language: "zh-Hans"}, false},
		{"upper language", preferencesResource{Language: "EN"}, true},
		{"root landing", preferencesResource{LandingPage: "/"}, false},
		{"nested landing", preferencesResource{LandingPage: "/settings/account"}, false},
		{"query landing", preferencesResource{LandingPage: "/apps?x=1"}, true},
		{"relative landing", preferencesResource{LandingPage: "apps"}, true},
	}
	for _, tt := range tests {
		if got := preferencesProblem(tt.in) != ""; got != tt.bad {
			t.Errorf("%s: problem = %v, want %v", tt.name, got, tt.bad)
		}
	}
}
