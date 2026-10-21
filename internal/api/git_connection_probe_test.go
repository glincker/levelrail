package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/bitbucketserver"
	"github.com/GLINCKER/levelrail/internal/gitprovider"
)

func TestConnProbe_TypedFailures(t *testing.T) {
	t.Setenv(gitprovider.AllowPrivateEnv, "true")
	bbs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/plugins/servlet/applinks/whoami" && r.Header.Get("Authorization") == "Bearer good":
			_, _ = w.Write([]byte("jane"))
		case r.URL.Path == "/plugins/servlet/applinks/whoami":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/rest/api/1.0/users/jane":
			_, _ = w.Write([]byte(`{"id":42}`))
		case r.URL.Path == "/rest/api/1.0/application-properties":
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer bbs.Close()
	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer tlsSrv.Close()
	closed := httptest.NewServer(http.NotFoundHandler())
	closedURL := closed.URL
	closed.Close()

	rt, db := newGitConnTestRouter(t, newFakeGitConnSecrets())
	rt.bitbucketServerClient = bitbucketserver.NewClient()
	cookie := loginTestSession(t, rt, db)

	tests := []struct {
		name, provider, url, token, wantKind string
		wantOK                               bool
	}{
		{"ok with token", "bitbucket_server", bbs.URL, "good", probeOK, true},
		{"reachable without token", "bitbucket_server", bbs.URL, "", probeOK, true},
		{"bad token", "bitbucket_server", bbs.URL, "wrong", probeBadToken, false},
		{"bad url scheme", "forgejo", "ftp://git.corp", "", probeBadURL, false},
		{"not this kind of server", "forgejo", bbs.URL, "", probeBadURL, false},
		{"tls", "bitbucket_server", tlsSrv.URL, "good", probeTLS, false},
		{"unreachable", "bitbucket_server", closedURL, "good", probeUnreachable, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"provider":"` + tc.provider + `","instance_url":"` + tc.url + `","token":"` + tc.token + `"}`
			rec := gcRequest(t, rt, cookie, http.MethodPost, "/api/v1/git-connections/test", body)
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			var got gitProbeResult
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Kind != tc.wantKind || got.OK != tc.wantOK {
				t.Fatalf("got %+v, want kind %s ok %v", got, tc.wantKind, tc.wantOK)
			}
			if !got.OK && got.Fix == "" {
				t.Errorf("failure %q has no fix hint", got.Kind)
			}
			if strings.Contains(rec.Body.String(), tc.token) && tc.token != "" {
				t.Errorf("response leaks the token")
			}
		})
	}
}
