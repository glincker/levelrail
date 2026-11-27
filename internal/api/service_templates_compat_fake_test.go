package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeCompatStore struct{ rows map[string]string }

func (f *fakeCompatStore) UpsertTemplateCompat(_ context.Context, _, _, class, detail string) error {
	f.rows[class] = detail
	return nil
}
func (f *fakeCompatStore) ClearTemplateCompat(context.Context, string, string) error { return nil }
func (f *fakeCompatStore) ListTemplateCompat(context.Context) ([]store.TemplateCompat, error) {
	return nil, nil
}

func TestRecordTemplateCompatIgnoresCallerText(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		caller string
		want   bool
	}{
		{"name conflict", http.StatusConflict, `{"error":"an app named privileged-x already exists"}`, "privileged-x", false},
		{"caller name in refusal", http.StatusBadRequest, `{"error":"invalid app name privileged-x"}`, "privileged-x", false},
		{"real refusal", http.StatusBadRequest, `{"error":"capability class is disabled: net_admin is off in runtime settings"}`, "web", true},
	}
	for _, tc := range cases {
		fs := &fakeCompatStore{rows: map[string]string{}}
		rt := &Router{templateCompat: fs, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
		rec := &compatRecorder{ResponseWriter: httptest.NewRecorder(), status: tc.status, body: []byte(tc.body), callerName: tc.caller}
		rt.recordTemplateCompat(httptest.NewRequest(http.MethodPost, "/x", nil), "t", rec)
		if got := len(fs.rows) > 0; got != tc.want {
			t.Errorf("%s: recorded = %v, want %v (%v)", tc.name, got, tc.want, fs.rows)
		}
		for _, d := range fs.rows {
			if strings.Contains(d, tc.caller) {
				t.Errorf("%s: detail holds caller text: %q", tc.name, d)
			}
		}
	}
}
