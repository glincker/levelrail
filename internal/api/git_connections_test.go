package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/gitlabapp"
)

func gcRequest(t *testing.T, rt *Router, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
	return rec
}

func decodeConn(t *testing.T, rec *httptest.ResponseRecorder) gitConnectionResource {
	t.Helper()
	var c gitConnectionResource
	if err := json.Unmarshal(rec.Body.Bytes(), &c); err != nil {
		t.Fatalf("decode connection: %v: %s", err, rec.Body.String())
	}
	return c
}

func listConns(t *testing.T, rt *Router, cookie *http.Cookie, provider string) []gitConnectionResource {
	t.Helper()
	rec := gcRequest(t, rt, cookie, http.MethodGet, "/api/v1/git-connections?provider="+provider, "")
	var out []gitConnectionResource
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &out) != nil {
		t.Fatalf("list connections: status %d body %s", rec.Code, rec.Body.String())
	}
	return out
}

func createGitLabConn(t *testing.T, rt *Router, cookie *http.Cookie, instance, clientID string) gitConnectionResource {
	t.Helper()
	body := `{"provider":"gitlab","instance_url":"` + instance + `","client_id":"` + clientID + `","client_secret":"s3cret"}`
	rec := gcRequest(t, rt, cookie, http.MethodPost, "/api/v1/git-connections", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create connection: status %d body %s", rec.Code, rec.Body.String())
	}
	return decodeConn(t, rec)
}

// authorizeConn drives the real connect redirect and callback for one connection.
func authorizeConn(t *testing.T, rt *Router, cookie *http.Cookie, connectionID, code string) *httptest.ResponseRecorder {
	t.Helper()
	path := "/api/v1/gitlab-app/connect"
	if connectionID != "" {
		path += "?connection_id=" + url.QueryEscape(connectionID)
	}
	start := gcRequest(t, rt, cookie, http.MethodGet, path, "")
	if start.Code != http.StatusFound {
		t.Fatalf("connect: status %d body %s", start.Code, start.Body.String())
	}
	loc, err := url.Parse(start.Header().Get("Location"))
	if err != nil || loc.Query().Get("state") == "" {
		t.Fatalf("connect redirect %q has no state: %v", start.Header().Get("Location"), err)
	}
	return gcRequest(t, rt, cookie, http.MethodGet, "/api/v1/gitlab-app/callback?code="+code+"&state="+loc.Query().Get("state"), "")
}

func twoAccountGitLabFake() *fakeGitLabAppClient {
	exp := time.Now().Add(2 * time.Hour)
	return &fakeGitLabAppClient{
		exchangeByCode: map[string]gitlabapp.Tokens{
			"code-ann":  {AccessToken: "at-ann", RefreshToken: "rt-ann", ExpiresAt: exp},
			"code-bob":  {AccessToken: "at-bob", RefreshToken: "rt-bob", ExpiresAt: exp},
			"code-ann2": {AccessToken: "at-ann2", RefreshToken: "rt-ann2", ExpiresAt: exp},
		},
		accountByToken: map[string]gitlabapp.Account{
			"at-ann":  {ID: "1", Login: "ann"},
			"at-ann2": {ID: "1", Login: "ann"},
			"at-bob":  {ID: "2", Login: "bob"},
		},
	}
}

func newOAuthRouter(t *testing.T, fake GitLabAppClient) (*Router, *fakeGitConnSecrets, *http.Cookie) {
	t.Helper()
	secrets := newFakeGitConnSecrets()
	rt, db := newGitConnTestRouter(t, secrets)
	rt.gitlabAppClient = fake
	setPrimaryDomain(t, db)
	return rt, secrets, loginTestSession(t, rt, db)
}

func TestGitConnections_CreateAddsInsteadOfReplacing(t *testing.T) {
	rt, db := newGitConnTestRouter(t, newFakeGitConnSecrets())
	cookie := loginTestSession(t, rt, db)

	first := createGitLabConn(t, rt, cookie, "https://gitlab.com", "cid-a")
	second := createGitLabConn(t, rt, cookie, "https://git.corp.example/", "cid-b")
	rec := gcRequest(t, rt, cookie, http.MethodPost, "/api/v1/git-connections", `{"provider":"gitea","instance_url":"https://git.home.lan","client_id":"g","client_secret":"x"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("gitea create: %d %s", rec.Code, rec.Body.String())
	}

	conns := listConns(t, rt, cookie, "gitlab")
	if len(conns) != 2 {
		t.Fatalf("gitlab connections = %d, want 2: a second account must not replace the first", len(conns))
	}
	if !first.IsDefault || second.IsDefault {
		t.Errorf("default flags: first %v second %v, want the first connection to be the default", first.IsDefault, second.IsDefault)
	}
	if second.Host != "git.corp.example" || second.InstanceURL != "https://git.corp.example" {
		t.Errorf("second = %+v, want host and trimmed url of the self-managed instance", second)
	}
	if got := listConns(t, rt, cookie, "gitea"); len(got) != 1 {
		t.Errorf("gitea connections = %d, want 1", len(got))
	}
	if first.Authorized || !strings.Contains(first.AuthorizePath, "/api/v1/gitlab-app/connect?connection_id="+first.ID) {
		t.Errorf("first = %+v, want unauthorized with an authorize path", first)
	}
}

func TestGitConnections_CreateValidation(t *testing.T) {
	tests := []struct {
		name, body string
		want       int
	}{
		{"unknown provider", `{"provider":"gitgud","instance_url":"https://x.example","client_id":"a","client_secret":"b"}`, http.StatusBadRequest},
		{"missing instance", `{"provider":"gitlab","client_id":"a","client_secret":"b"}`, http.StatusBadRequest},
		{"bad scheme", `{"provider":"gitlab","instance_url":"ftp://x.example","client_id":"a","client_secret":"b"}`, http.StatusBadRequest},
		{"missing client id", `{"provider":"gitlab","instance_url":"https://x.example","client_secret":"b"}`, http.StatusBadRequest},
		{"missing secret", `{"provider":"gitea","instance_url":"https://x.example","client_id":"a"}`, http.StatusBadRequest},
		{"bitbucket needs no instance", `{"provider":"bitbucket","client_id":"key","client_secret":"sec"}`, http.StatusCreated},
		{"unknown reuse target", `{"provider":"gitlab","reuse_connection_id":"gc_nope"}`, http.StatusBadRequest},
		{"bad json", `{`, http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt, db := newGitConnTestRouter(t, newFakeGitConnSecrets())
			cookie := loginTestSession(t, rt, db)
			if rec := gcRequest(t, rt, cookie, http.MethodPost, "/api/v1/git-connections", tc.body); rec.Code != tc.want {
				t.Fatalf("status = %d, want %d, body %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestGitConnections_CreateWithoutMasterKeyIs501(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := gcRequest(t, rt, cookie, http.MethodPost, "/api/v1/git-connections", `{"provider":"gitlab","instance_url":"https://x.example","client_id":"a","client_secret":"b"}`)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", rec.Code)
	}
}

func TestGitConnections_ReuseCopiesApplicationAndSecret(t *testing.T) {
	secrets := newFakeGitConnSecrets()
	rt, db := newGitConnTestRouter(t, secrets)
	cookie := loginTestSession(t, rt, db)
	first := createGitLabConn(t, rt, cookie, "https://gitlab.com", "cid-a")

	rec := gcRequest(t, rt, cookie, http.MethodPost, "/api/v1/git-connections", `{"provider":"gitlab","reuse_connection_id":"`+first.ID+`","label":"Work"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	second := decodeConn(t, rec)
	if second.ID == first.ID || second.ClientID != "cid-a" || second.Host != "gitlab.com" || second.Label != "Work" {
		t.Fatalf("second = %+v", second)
	}
	stored, err := db.GetGitConnection(t.Context(), second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := secrets.Resolve(t.Context(), stored.SecretsKey, gitConnClientSecretKey); got != "s3cret" {
		t.Errorf("reused client secret = %q, want it copied under the new connection's own secrets key", got)
	}
	if strings.Contains(rec.Body.String(), "s3cret") {
		t.Error("response leaks the client secret")
	}
}

func TestGitConnectOAuth_SecondAccountAddsConnectionAndRecordsAccount(t *testing.T) {
	rt, secrets, cookie := newOAuthRouter(t, twoAccountGitLabFake())
	a := createGitLabConn(t, rt, cookie, "https://gitlab.com", "cid-a")
	if rec := authorizeConn(t, rt, cookie, a.ID, "code-ann"); rec.Code != http.StatusFound {
		t.Fatalf("callback ann: %d %s", rec.Code, rec.Body.String())
	}
	rec := gcRequest(t, rt, cookie, http.MethodPost, "/api/v1/git-connections", `{"provider":"gitlab","reuse_connection_id":"`+a.ID+`"}`)
	b := decodeConn(t, rec)
	if cb := authorizeConn(t, rt, cookie, b.ID, "code-bob"); cb.Code != http.StatusFound {
		t.Fatalf("callback bob: %d %s", cb.Code, cb.Body.String())
	} else if strings.Contains(cb.Header().Get("Location"), "at-bob") {
		t.Error("redirect leaks an access token")
	}

	conns := listConns(t, rt, cookie, "gitlab")
	if len(conns) != 2 {
		t.Fatalf("connections = %d, want 2", len(conns))
	}
	byID := map[string]gitConnectionResource{}
	for _, c := range conns {
		byID[c.ID] = c
	}
	if got := byID[a.ID]; got.AccountName != "ann" || !got.Authorized || got.Label != "ann @ gitlab.com" || got.Scopes != "api" {
		t.Errorf("first connection = %+v", got)
	}
	if got := byID[b.ID]; got.AccountName != "bob" || !got.Authorized || got.Label != "bob @ gitlab.com" {
		t.Errorf("second connection = %+v", got)
	}
	stored, _ := connStore(rt).GetGitConnection(t.Context(), a.ID)
	if tok, _ := secrets.Resolve(t.Context(), stored.SecretsKey, gitConnAccessTokenKey); tok != "at-ann" {
		t.Errorf("first connection token = %q, want at-ann untouched by the second account", tok)
	}
}

func connStore(rt *Router) GitConnectionsStore { return rt.gitConns }

func TestGitConnectOAuth_ReconnectSameAccountUpdatesInsteadOfDuplicating(t *testing.T) {
	tests := []struct {
		name string
		// viaNewConnection reconnects through a freshly added connection on the
		// same OAuth application instead of the connection id.
		viaNewConnection bool
	}{
		{"explicit connection id", false},
		{"add another account but same login", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt, secrets, cookie := newOAuthRouter(t, twoAccountGitLabFake())
			a := createGitLabConn(t, rt, cookie, "https://gitlab.com", "cid-a")
			authorizeConn(t, rt, cookie, a.ID, "code-ann")

			target := a.ID
			if tc.viaNewConnection {
				rec := gcRequest(t, rt, cookie, http.MethodPost, "/api/v1/git-connections", `{"provider":"gitlab","reuse_connection_id":"`+a.ID+`"}`)
				target = decodeConn(t, rec).ID
			}
			if rec := authorizeConn(t, rt, cookie, target, "code-ann2"); rec.Code != http.StatusFound {
				t.Fatalf("callback: %d %s", rec.Code, rec.Body.String())
			}

			conns := listConns(t, rt, cookie, "gitlab")
			if len(conns) != 1 || conns[0].ID != a.ID {
				t.Fatalf("connections = %+v, want only the original connection", conns)
			}
			stored, _ := connStore(rt).GetGitConnection(t.Context(), a.ID)
			if tok, _ := secrets.Resolve(t.Context(), stored.SecretsKey, gitConnAccessTokenKey); tok != "at-ann2" {
				t.Errorf("token = %q, want the reconnect's at-ann2", tok)
			}
		})
	}
}

func TestGitConnectOAuth_SameLoginThroughAnotherApplicationStaysSeparate(t *testing.T) {
	rt, _, cookie := newOAuthRouter(t, twoAccountGitLabFake())
	a := createGitLabConn(t, rt, cookie, "https://gitlab.com", "cid-personal")
	authorizeConn(t, rt, cookie, a.ID, "code-ann")
	b := createGitLabConn(t, rt, cookie, "https://gitlab.com", "cid-work")
	authorizeConn(t, rt, cookie, b.ID, "code-ann2")

	if got := len(listConns(t, rt, cookie, "gitlab")); got != 2 {
		t.Fatalf("connections = %d, want 2: two OAuth applications are two connections", got)
	}
}

func TestGitConnectOAuth_ReconnectWithDifferentAccountAddsAConnectionAndKeepsTheOriginal(t *testing.T) {
	rt, secrets, cookie := newOAuthRouter(t, twoAccountGitLabFake())
	a := createGitLabConn(t, rt, cookie, "https://gitlab.com", "cid-a")
	authorizeConn(t, rt, cookie, a.ID, "code-ann")

	rec := authorizeConn(t, rt, cookie, a.ID, "code-bob")
	if rec.Code != http.StatusFound {
		t.Fatalf("callback: %d %s", rec.Code, rec.Body.String())
	}
	conns := listConns(t, rt, cookie, "gitlab")
	if len(conns) != 2 {
		t.Fatalf("connections = %d, want 2", len(conns))
	}
	stored, _ := connStore(rt).GetGitConnection(t.Context(), a.ID)
	if tok, _ := secrets.Resolve(t.Context(), stored.SecretsKey, gitConnAccessTokenKey); tok != "at-ann" {
		t.Errorf("original connection token = %q, want at-ann: another login must not take it over", tok)
	}
	if stored.AccountName != "ann" {
		t.Errorf("original account = %q, want ann", stored.AccountName)
	}
}

func TestGitConnectOAuth_StartPicksNewestUnauthorizedWithoutAnID(t *testing.T) {
	rt, _, cookie := newOAuthRouter(t, twoAccountGitLabFake())
	a := createGitLabConn(t, rt, cookie, "https://gitlab.com", "cid-a")
	authorizeConn(t, rt, cookie, a.ID, "code-ann")
	createGitLabConn(t, rt, cookie, "https://git.corp.example", "cid-b")

	start := gcRequest(t, rt, cookie, http.MethodGet, "/api/v1/gitlab-app/connect", "")
	if start.Code != http.StatusFound || !strings.HasPrefix(start.Header().Get("Location"), "https://git.corp.example/oauth/authorize?") || !strings.Contains(start.Header().Get("Location"), "client_id=cid-b") {
		t.Fatalf("status %d location %q, want the newest unauthorized connection's authorize page", start.Code, start.Header().Get("Location"))
	}
	if rec := gcRequest(t, rt, cookie, http.MethodGet, "/api/v1/gitlab-app/connect?connection_id=gc_unknown", ""); rec.Code != http.StatusConflict {
		t.Errorf("unknown connection id: status %d, want 409", rec.Code)
	}
}

func TestGitConnectOAuth_StartNeedsAPrimaryDomainAndConnection(t *testing.T) {
	secrets := newFakeGitConnSecrets()
	rt, db := newGitConnTestRouter(t, secrets)
	cookie := loginTestSession(t, rt, db)
	if rec := gcRequest(t, rt, cookie, http.MethodGet, "/api/v1/gitlab-app/connect", ""); rec.Code != http.StatusConflict {
		t.Errorf("no connection: status %d, want 409", rec.Code)
	}
	c := createGitLabConn(t, rt, cookie, "https://gitlab.com", "cid")
	if rec := gcRequest(t, rt, cookie, http.MethodGet, "/api/v1/gitlab-app/connect?connection_id="+c.ID, ""); rec.Code != http.StatusConflict {
		t.Errorf("no primary domain: status %d, want 409", rec.Code)
	}
}

func TestGitConnectOAuth_CallbackRejectsBadStateAndMissingParams(t *testing.T) {
	rt, _, cookie := newOAuthRouter(t, twoAccountGitLabFake())
	for _, q := range []string{"state=x", "code=abc", "code=abc&state=never-issued"} {
		if rec := gcRequest(t, rt, cookie, http.MethodGet, "/api/v1/gitlab-app/callback?"+q, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", q, rec.Code)
		}
	}
}

func TestGitConnectOAuth_StateIsSingleUseAndBoundToItsConnection(t *testing.T) {
	rt, _, cookie := newOAuthRouter(t, twoAccountGitLabFake())
	a := createGitLabConn(t, rt, cookie, "https://gitlab.com", "cid-a")
	b := createGitLabConn(t, rt, cookie, "https://git.corp.example", "cid-b")
	startA := gcRequest(t, rt, cookie, http.MethodGet, "/api/v1/gitlab-app/connect?connection_id="+a.ID, "")
	startB := gcRequest(t, rt, cookie, http.MethodGet, "/api/v1/gitlab-app/connect?connection_id="+b.ID, "")
	stateOf := func(rec *httptest.ResponseRecorder) string {
		loc, _ := url.Parse(rec.Header().Get("Location"))
		return loc.Query().Get("state")
	}
	if rec := gcRequest(t, rt, cookie, http.MethodGet, "/api/v1/gitlab-app/callback?code=code-bob&state="+stateOf(startB), ""); rec.Code != http.StatusFound {
		t.Fatalf("callback for the second attempt while the first is still pending: %d %s", rec.Code, rec.Body.String())
	}
	if rec := gcRequest(t, rt, cookie, http.MethodGet, "/api/v1/gitlab-app/callback?code=code-bob&state="+stateOf(startB), ""); rec.Code != http.StatusBadRequest {
		t.Errorf("replayed state: status %d, want 400", rec.Code)
	}
	if rec := gcRequest(t, rt, cookie, http.MethodGet, "/api/v1/gitlab-app/callback?code=code-ann&state="+stateOf(startA), ""); rec.Code != http.StatusFound {
		t.Errorf("first attempt must still complete: %d", rec.Code)
	}
}

func TestGitConnections_RenameAndSetDefault(t *testing.T) {
	rt, db := newGitConnTestRouter(t, newFakeGitConnSecrets())
	cookie := loginTestSession(t, rt, db)
	createGitLabConn(t, rt, cookie, "https://gitlab.com", "cid-a")
	b := createGitLabConn(t, rt, cookie, "https://git.corp.example", "cid-b")

	rec := gcRequest(t, rt, cookie, http.MethodPatch, "/api/v1/git-connections/"+b.ID, `{"label":"  Work GitLab "}`)
	if got := decodeConn(t, rec); rec.Code != http.StatusOK || got.Label != "Work GitLab" {
		t.Fatalf("rename: %d %+v", rec.Code, got)
	}
	for _, bad := range []string{`{"label":""}`, `{"label":"   "}`, `{"label":"` + strings.Repeat("x", 81) + `"}`, `{`} {
		if rec := gcRequest(t, rt, cookie, http.MethodPatch, "/api/v1/git-connections/"+b.ID, bad); rec.Code != http.StatusBadRequest {
			t.Errorf("rename %q: status %d, want 400", bad, rec.Code)
		}
	}
	if rec := gcRequest(t, rt, cookie, http.MethodPost, "/api/v1/git-connections/"+b.ID+"/default", ""); rec.Code != http.StatusOK {
		t.Fatalf("set default: %d %s", rec.Code, rec.Body.String())
	}
	conns := listConns(t, rt, cookie, "gitlab")
	for _, c := range conns {
		if want := c.ID == b.ID; c.IsDefault != want {
			t.Errorf("connection %s default = %v, want %v", c.ID, c.IsDefault, want)
		}
	}
	for _, path := range []string{"/api/v1/git-connections/gc_none", "/api/v1/git-connections/gc_none/default"} {
		method := http.MethodGet
		if strings.HasSuffix(path, "/default") {
			method = http.MethodPost
		}
		if rec := gcRequest(t, rt, cookie, method, path, ""); rec.Code != http.StatusNotFound {
			t.Errorf("%s %s: status %d, want 404", method, path, rec.Code)
		}
	}
}

func TestGitConnections_DisconnectKeepsOthersAndSourcesNeedReconnect(t *testing.T) {
	secrets := newFakeGitConnSecrets()
	rt, db := newGitConnTestRouter(t, secrets, WithGitSourceSecrets(newFakeGitSourceSecrets()))
	cookie := loginTestSession(t, rt, db)
	setPrimaryDomain(t, db)
	a := seedGitConn(t, db, secrets, gitProviderGitLab, "gitlab.com", "ann", true)
	b := seedGitConn(t, db, secrets, gitProviderGitLab, "git.corp.example", "bob", true)
	seedApp(t, db, "web")
	connectGitSource(t, rt, cookie, `{"repo_url":"https://git.corp.example/g/web.git","branch":"main"}`)
	if err := db.SetGitSourceConnection(t.Context(), "web", b.ID); err != nil {
		t.Fatal(err)
	}

	rec := gcRequest(t, rt, cookie, http.MethodDelete, "/api/v1/git-connections/"+b.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("disconnect: %d %s", rec.Code, rec.Body.String())
	}
	var out gitDisconnectResource
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out.AffectedSources) != 1 || out.AffectedSources[0] != "web" || !strings.Contains(out.Note, "needs reconnect") {
		t.Fatalf("response = %s, want the dependent source named with a reconnect note (err %v)", rec.Body.String(), err)
	}
	if got := listConns(t, rt, cookie, "gitlab"); len(got) != 1 || got[0].ID != a.ID || !got[0].Authorized {
		t.Fatalf("remaining connections = %+v, want only the untouched first account", got)
	}

	rec = gcRequest(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/git-source", "")
	var src gitSourceResource
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &src) != nil {
		t.Fatalf("source must survive the disconnect: %d %s", rec.Code, rec.Body.String())
	}
	if src.ConnectionID != b.ID || src.Connection == nil || src.Connection.State != gitSourceNeedsReconnect {
		t.Fatalf("source connection = %+v, want needs_reconnect", src.Connection)
	}
}

func TestGitConnectOAuth_NewConnectionOnSameHostReattachesOrphanedSources(t *testing.T) {
	secrets := newFakeGitConnSecrets()
	fake := twoAccountGitLabFake()
	rt, db := newGitConnTestRouter(t, secrets, WithGitSourceSecrets(newFakeGitSourceSecrets()))
	rt.gitlabAppClient = fake
	setPrimaryDomain(t, db)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	connectGitSource(t, rt, cookie, `{"repo_url":"https://gitlab.com/g/web.git","branch":"main"}`)
	if err := db.SetGitSourceConnection(t.Context(), "web", "gc_removed"); err != nil {
		t.Fatal(err)
	}

	c := createGitLabConn(t, rt, cookie, "https://gitlab.com", "cid-new")
	authorizeConn(t, rt, cookie, c.ID, "code-ann")

	gs, err := db.GetGitSource(t.Context(), "web")
	if err != nil || gs.ConnectionID != c.ID {
		t.Fatalf("source connection = %q, err %v; want the new connection %q", gs.ConnectionID, err, c.ID)
	}
}

func TestGitConnections_BackfillsAccountOfMigratedConnectionOnFirstUse(t *testing.T) {
	fake := &fakeGitLabAppClient{account: gitlabapp.Account{ID: "7", Login: "migrated"}}
	secrets := newFakeGitConnSecrets()
	rt, db := newGitConnTestRouter(t, secrets)
	rt.gitlabAppClient = fake
	c := seedGitConn(t, db, secrets, gitProviderGitLab, "gitlab.com", "", true)
	if err := db.RenameGitConnection(t.Context(), c.ID, "gitlab.com"); err != nil {
		t.Fatal(err)
	}

	if _, _, err := rt.gitConnToken(t.Context(), c.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := db.GetGitConnection(t.Context(), c.ID)
	if got.AccountID != "7" || got.AccountName != "migrated" || got.Label != "migrated @ gitlab.com" {
		t.Fatalf("connection = %+v, want account and label filled in after first use", got)
	}
}

func TestGitRepoList_MergesAccountsAndKeepsSameNamedReposApart(t *testing.T) {
	fake := &fakeGitLabAppClient{projectsByToken: map[string][]gitlabapp.Project{
		"at-ann": {{ID: 1, Name: "web", PathWithNamespace: "ann/web", HTTPURLToRepo: "https://gitlab.com/ann/web.git"}},
		"at-bob": {{ID: 2, Name: "web", PathWithNamespace: "bob/web", HTTPURLToRepo: "https://git.corp.example/bob/web.git"}, {ID: 3, Name: "api", PathWithNamespace: "bob/api"}},
	}}
	secrets := newFakeGitConnSecrets()
	rt, db := newGitConnTestRouter(t, secrets)
	rt.gitlabAppClient = fake
	rt.repoCache = newRepoCacheForTest()
	cookie := loginTestSession(t, rt, db)
	a := seedGitConn(t, db, secrets, gitProviderGitLab, "gitlab.com", "ann", true)
	b := seedGitConn(t, db, secrets, gitProviderGitLab, "git.corp.example", "bob", true)

	code, _, all, _ := getRepoList(t, rt, cookie, "")
	if code != http.StatusOK || all.Total != 3 || len(all.Connections) != 2 {
		t.Fatalf("code %d total %d connections %d, want 3 repos from 2 accounts", code, all.Total, len(all.Connections))
	}
	byConn := map[string]int{}
	for _, r := range all.Items {
		byConn[r.ConnectionID]++
		if r.Provider != gitProviderGitLab || r.Account == "" || r.ConnectionLabel == "" || r.Owner == "" {
			t.Errorf("row %+v must say which account and owner it came from", r)
		}
	}
	if byConn[a.ID] != 1 || byConn[b.ID] != 2 {
		t.Errorf("rows per connection = %v", byConn)
	}
	_, _, onlyA, _ := getRepoList(t, rt, cookie, "provider=gitlab&connection="+a.ID)
	if onlyA.Total != 1 || onlyA.Items[0].FullName != "ann/web" {
		t.Errorf("filtered to the first account: %+v", onlyA.Items)
	}
	_, _, search, _ := getRepoList(t, rt, cookie, "search=web")
	if search.Total != 2 {
		t.Errorf("search web total = %d, want both same-named repos", search.Total)
	}
	if fake.listCalls.Load() != 2 {
		t.Errorf("listings = %d, want one per connection", fake.listCalls.Load())
	}
}

func TestGitRepoList_DisconnectDropsOnlyThatConnectionsCache(t *testing.T) {
	fake := &fakeGitLabAppClient{projectsByToken: map[string][]gitlabapp.Project{
		"at-ann": {{ID: 1, Name: "a", PathWithNamespace: "ann/a"}},
		"at-bob": {{ID: 2, Name: "b", PathWithNamespace: "bob/b"}},
	}}
	secrets := newFakeGitConnSecrets()
	rt, db := newGitConnTestRouter(t, secrets)
	rt.gitlabAppClient = fake
	rt.repoCache = newRepoCacheForTest()
	cookie := loginTestSession(t, rt, db)
	a := seedGitConn(t, db, secrets, gitProviderGitLab, "gitlab.com", "ann", true)
	seedGitConn(t, db, secrets, gitProviderGitLab, "git.corp.example", "bob", true)

	getRepoList(t, rt, cookie, "provider=gitlab")
	if rec := gcRequest(t, rt, cookie, http.MethodDelete, "/api/v1/git-connections/"+a.ID, ""); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	before := fake.listCalls.Load()
	_, _, res, _ := getRepoList(t, rt, cookie, "provider=gitlab")
	if res.Total != 1 || res.Items[0].FullName != "bob/b" {
		t.Fatalf("after disconnect: %+v", res.Items)
	}
	if fake.listCalls.Load() != before {
		t.Errorf("the remaining account's list was re-fetched (%d -> %d), its cache must survive", before, fake.listCalls.Load())
	}
}

func TestGitConnections_UseAsSourceRecordsConnectionAndHooksWithItsToken(t *testing.T) {
	fake := &fakeGitLabAppClient{
		project: gitlabapp.Project{ID: 7, Name: "web", PathWithNamespace: "bob/web", HTTPURLToRepo: "https://gitlab.com/bob/web.git", DefaultBranch: "trunk"},
	}
	secrets := newFakeGitConnSecrets()
	gitSecrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitLabAndGitSource(t, secrets, fake, gitSecrets)
	cookie := loginTestSession(t, rt, db)
	setPrimaryDomain(t, db)
	seedGitConn(t, db, secrets, gitProviderGitLab, "gitlab.com", "ann", true)
	b := seedGitConn(t, db, secrets, gitProviderGitLab, "gitlab.com", "bob", true)
	seedApp(t, db, "web")

	rec := gcRequest(t, rt, cookie, http.MethodPost, "/api/v1/git-connections/"+b.ID+"/use-as-source", `{"app_name":"web","repo":"7"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var res gitSourceResource
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if res.ConnectionID != b.ID || res.Branch != "trunk" {
		t.Errorf("resource = %+v", res)
	}
	gs, err := db.GetGitSource(t.Context(), "web")
	if err != nil || gs.ConnectionID != b.ID {
		t.Fatalf("stored source connection = %q, err %v; want %q", gs.ConnectionID, err, b.ID)
	}
	if fake.createHookAccess != "at-bob" {
		t.Errorf("webhook registered with token %q, want the chosen connection's at-bob (not the default account's)", fake.createHookAccess)
	}
	assertGitSourceWebhookRegistered(t, gitSecrets, "web", fake.createHookCall, fake.createHookURL, fake.createHookTok)
}

func TestGitConnections_UseAsSourceValidation(t *testing.T) {
	secrets := newFakeGitConnSecrets()
	rt, db := newTestRouterWithGitLabAndGitSource(t, secrets, &fakeGitLabAppClient{}, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	c := seedGitConn(t, db, secrets, gitProviderGitLab, "gitlab.com", "ann", true)
	idle := seedGitConn(t, db, secrets, gitProviderGitLab, "git.corp.example", "bob", false)
	seedApp(t, db, "web")

	tests := []struct {
		name, id, body string
		want           int
	}{
		{"missing repo", c.ID, `{"app_name":"web"}`, http.StatusBadRequest},
		{"unknown app", c.ID, `{"app_name":"nope","repo":"7"}`, http.StatusNotFound},
		{"unknown connection", "gc_none", `{"app_name":"web","repo":"7"}`, http.StatusNotFound},
		{"unauthorized connection", idle.ID, `{"app_name":"web","repo":"7"}`, http.StatusConflict},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if rec := gcRequest(t, rt, cookie, http.MethodPost, "/api/v1/git-connections/"+tc.id+"/use-as-source", tc.body); rec.Code != tc.want {
				t.Fatalf("status %d, want %d, body %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestGitConnections_BranchesUseTheNamedConnection(t *testing.T) {
	fake := &fakeGitLabAppClient{branches: []gitlabapp.Branch{{Name: "main", CommitSHA: "abc"}}}
	secrets := newFakeGitConnSecrets()
	rt, db := newGitConnTestRouter(t, secrets)
	rt.gitlabAppClient = fake
	cookie := loginTestSession(t, rt, db)
	c := seedGitConn(t, db, secrets, gitProviderGitLab, "gitlab.com", "ann", true)

	rec := gcRequest(t, rt, cookie, http.MethodGet, "/api/v1/git-connections/"+c.ID+"/branches?repo=7", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"main"`) {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if rec := gcRequest(t, rt, cookie, http.MethodGet, "/api/v1/git-connections/"+c.ID+"/branches", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("missing repo: status %d, want 400", rec.Code)
	}
}

func TestGitSourceWebhookRegistrationUsesTheSourcesOwnConnection(t *testing.T) {
	fake := &fakeGitLabAppClient{projectsByToken: map[string][]gitlabapp.Project{
		"at-ann": {{ID: 1, PathWithNamespace: "ann/web"}},
		"at-bob": {{ID: 2, PathWithNamespace: "bob/web"}},
	}}
	secrets := newFakeGitConnSecrets()
	rt, db := newTestRouterWithGitLabAndGitSource(t, secrets, fake, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	setPrimaryDomain(t, db)
	seedGitConn(t, db, secrets, gitProviderGitLab, "gitlab.com", "ann", true)
	b := seedGitConn(t, db, secrets, gitProviderGitLab, "gitlab.com", "bob", true)
	seedApp(t, db, "web")
	connectGitSource(t, rt, cookie, `{"repo_url":"https://gitlab.com/bob/web.git","branch":"main"}`)
	if err := db.SetGitSourceConnection(t.Context(), "web", b.ID); err != nil {
		t.Fatal(err)
	}

	rec := gcRequest(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/git-source/webhook/register", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if !fake.createHookCall || fake.createHookAccess != "at-bob" {
		t.Fatalf("hook called %v with token %q, want bob's token for bob's source", fake.createHookCall, fake.createHookAccess)
	}
}

func TestGitConnectionRoutes_AbilityTiers(t *testing.T) {
	rt, db := newTestRouter(t)
	const readSensitive = "tok_gc_rs"                 //nolint:gosec // fixture id
	const plaintext = "git-conn-read-sensitive-token" //nolint:gosec // fake fixture, not a real credential
	rootOnly := []providerRouteCase{
		{method: http.MethodPost, path: "/api/v1/git-connections", body: `{}`},
		{method: http.MethodPatch, path: "/api/v1/git-connections/gc_x", body: `{"label":"x"}`},
		{method: http.MethodDelete, path: "/api/v1/git-connections/gc_x"},
		{method: http.MethodPost, path: "/api/v1/git-connections/gc_x/default"},
		{method: http.MethodGet, path: "/api/v1/gitlab-app/connect"},
		{method: http.MethodGet, path: "/api/v1/bitbucket-app/callback?code=a&state=b"},
		{method: http.MethodGet, path: "/api/v1/gitea-app/connect"},
	}
	assertProviderRoutesRequireAuth(t, rt, append(rootOnly,
		providerRouteCase{method: http.MethodGet, path: "/api/v1/git-connections"},
		providerRouteCase{method: http.MethodGet, path: "/api/v1/git-connections/gc_x"},
	))
	assertProviderRoutesForbiddenForAbilities(t, rt, db, readSensitive, plaintext, []string{AbilityReadSensitive, AbilityWriteSensitive}, rootOnly)

	for _, path := range []string{"/api/v1/git-connections", "/api/v1/git-connections/gc_x"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+plaintext)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		if rec.Code == http.StatusForbidden || rec.Code == http.StatusUnauthorized {
			t.Errorf("GET %s with read_sensitive: status %d, want it reachable", path, rec.Code)
		}
	}
}

func TestGitConnections_ListReportsNoCredentials(t *testing.T) {
	secrets := newFakeGitConnSecrets()
	rt, db := newGitConnTestRouter(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedGitConn(t, db, secrets, gitProviderGitLab, "gitlab.com", "ann", true)
	rec := gcRequest(t, rt, cookie, http.MethodGet, "/api/v1/git-connections", "")
	for _, secret := range []string{"csecret", "at-ann", "rt-ann"} {
		if strings.Contains(rec.Body.String(), secret) {
			t.Errorf("list response leaks %q: %s", secret, rec.Body.String())
		}
	}
}
