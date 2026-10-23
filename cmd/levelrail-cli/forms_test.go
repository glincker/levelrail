package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/clitree"
)

func formsServer(t *testing.T, seen *[]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*seen = append(*seen, r.Method+" "+r.URL.RequestURI()+" "+string(body))
		switch {
		case strings.HasSuffix(r.URL.Path, "/export"):
			w.Header().Set("Content-Type", "text/csv")
			_, _ = w.Write([]byte("id,created_at,spam,email\n1,2026-10-20T10:00:00Z,false,a@b.c\n"))
		case strings.Contains(r.URL.Path, "/submissions/"):
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/submissions"):
			_, _ = w.Write([]byte(`{"submissions":[{"id":7,"fields":{"email":"a@b.c"},"spam":true,"created_at":"2026-10-20T10:00:00Z"}],"next_before":0}`))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"forms":[{"id":"frm_1","name":"contact","endpoint":"https://cp.test/api/v1/forms/web/contact","snippet":"<form></form>","submissions":3,"spam_submissions":1}],"retention_days":90}`))
		default:
			_, _ = w.Write([]byte(`{"id":"frm_1","name":"contact","endpoint":"https://cp.test/api/v1/forms/web/contact","snippet":"<form action=x></form>"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_Forms(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
		call string
	}{
		{"list", []string{"forms", "list", "web"}, "contact", "GET /api/v1/apps/web/forms "},
		{"create", []string{"forms", "create", "web", "contact", "--redirect-url", "https://s.test/ok", "--notify", "ch1, ch2"}, "<form action=x>", `"name":"contact","redirect_url":"https://s.test/ok","notify_channel_ids":["ch1","ch2"]`},
		{"update only redirect", []string{"forms", "update", "web", "contact", "--redirect-url", ""}, "saved", `PUT /api/v1/apps/web/forms/contact {"redirect_url":""}`},
		{"snippet", []string{"forms", "snippet", "web", "contact"}, "<form></form>", "GET /api/v1/apps/web/forms "},
		{"submissions with filters", []string{"forms", "submissions", "web", "contact", "--spam", "true", "--search", "a b"}, "[spam]", "q=a+b&spam=true"},
		{"mark spam", []string{"forms", "spam", "web", "contact", "7"}, "spam done", `PUT /api/v1/apps/web/forms/contact/submissions/7/spam {"spam":true}`},
		{"unspam", []string{"forms", "unspam", "web", "contact", "7"}, "unspam done", `{"spam":false}`},
		{"remove", []string{"forms", "remove", "web", "contact", "7"}, "remove done", "DELETE /api/v1/apps/web/forms/contact/submissions/7"},
		{"export", []string{"forms", "export", "web", "contact"}, "id,created_at,spam,email", "/forms/contact/export"},
		{"delete", []string{"forms", "delete", "web", "contact"}, "deleted", "DELETE /api/v1/apps/web/forms/contact "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seen []string
			srv := formsServer(t, &seen)
			stdout, _ := runCLIExpectOK(t, append(tt.args, "--api-url", srv.URL))
			if !strings.Contains(stdout, tt.want) {
				t.Errorf("stdout %q missing %q", stdout, tt.want)
			}
			if !strings.Contains(strings.Join(seen, "\n"), tt.call) {
				t.Errorf("calls %v missing %q", seen, tt.call)
			}
		})
	}
}

func TestRun_FormsUsageErrors(t *testing.T) {
	for _, args := range [][]string{{"forms"}, {"forms", "create", "web"}, {"forms", "spam", "web", "contact"}, {"forms", "nope"}} {
		expectCacheUsageError(t, args)
	}
}

func TestFormsIsInTheCompletionTree(t *testing.T) {
	node := clitree.Tree["forms"]
	if node == nil {
		t.Fatal("forms missing from the completion tree")
	}
	for _, v := range []string{"list", "create", "update", "snippet", "delete", "submissions", "spam", "unspam", "remove", "export"} {
		if _, ok := node.Subs[v]; !ok {
			t.Errorf("forms %s missing", v)
		}
	}
}
