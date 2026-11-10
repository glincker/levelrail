package api

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/appexport"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestResourceNotesSetGetSearchCloneExport(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedV2Apps(t, db)
	t.Setenv("APP_RESOURCE_DESCRIPTION_MAX_CHARS", "40")
	t.Setenv("APP_RESOURCE_NOTES_MAX_BYTES", "2048")

	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}

	notes := "Owner: payments team\n\n<script>alert(1)</script> [runbook](https://runbooks.example/web)"
	body, _ := json.Marshal(map[string]string{"description": "  Checkout frontend  ", "notes": notes})
	rec := do(http.MethodPut, "/api/v1/apps/web/notes", string(body))
	if rec.Code != http.StatusOK {
		t.Fatalf("put: %d %s", rec.Code, rec.Body.String())
	}
	var got resourceNotesResource
	if err := json.Unmarshal(do(http.MethodGet, "/api/v1/apps/web/notes", "").Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Description != "Checkout frontend" || got.Notes != notes {
		t.Fatalf("get = %+v", got)
	}
	// An omitted field keeps its stored value.
	if rec := do(http.MethodPut, "/api/v1/apps/web/notes", `{"description":"Checkout web"}`); !strings.Contains(rec.Body.String(), "runbooks.example") {
		t.Fatalf("partial put dropped notes: %s", rec.Body.String())
	}

	rec = do(http.MethodGet, "/api/v1/apps?q=payments", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"description":"Checkout web"`) || strings.Contains(rec.Body.String(), `"worker"`) {
		t.Fatalf("search by notes: %d %s", rec.Code, rec.Body.String())
	}

	rec = do(http.MethodGet, "/api/v1/apps/web/export", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("export: %d %s", rec.Code, rec.Body.String())
	}
	exported := notesTarEntry(t, rec.Body.Bytes(), appexport.NotesFileName)
	if d, n := appexport.ParseNotes(exported); d != "Checkout web" || n != notes {
		t.Fatalf("export round trip = %q %q", d, n)
	}

	if rec := do(http.MethodPost, "/api/v1/apps/web/clone", `{"new_name":"web-copy"}`); rec.Code != http.StatusCreated {
		t.Fatalf("clone: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodGet, "/api/v1/apps/web-copy/notes", ""); !strings.Contains(rec.Body.String(), "payments team") {
		t.Fatalf("clone lost notes: %s", rec.Body.String())
	}
}

func TestResourceNotesValidationAndCaps(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedV2Apps(t, db)
	t.Setenv("APP_RESOURCE_DESCRIPTION_MAX_CHARS", "10")
	t.Setenv("APP_RESOURCE_NOTES_MAX_BYTES", "1024")

	cases := []struct {
		name string
		path string
		body string
		want int
	}{
		{"description over cap", "/api/v1/apps/web/notes", `{"description":"eleven chars"}`, http.StatusBadRequest},
		{"description with newline", "/api/v1/apps/web/notes", `{"description":"a\nb"}`, http.StatusBadRequest},
		{"notes over cap", "/api/v1/apps/web/notes", `{"notes":"` + strings.Repeat("x", 1025) + `"}`, http.StatusRequestEntityTooLarge},
		{"body far over cap", "/api/v1/apps/web/notes", `{"notes":"` + strings.Repeat("x", 1<<20) + `"}`, http.StatusRequestEntityTooLarge},
		{"notes with NUL", "/api/v1/apps/web/notes", `{"notes":"a\u0000b"}`, http.StatusBadRequest},
		{"notes with OSC 52 escape", "/api/v1/apps/web/notes", `{"notes":"a\u001b]52;c;ZXZpbA==\u0007b"}`, http.StatusBadRequest},
		{"notes with C1 CSI", "/api/v1/apps/web/notes", `{"notes":"a\u009b2Jb"}`, http.StatusBadRequest},
		{"notes with bidi override", "/api/v1/apps/web/notes", `{"notes":"a\u202eb"}`, http.StatusBadRequest},
		{"notes with bidi isolate", "/api/v1/apps/web/notes", `{"notes":"a\u2067b"}`, http.StatusBadRequest},
		{"description with zero width", "/api/v1/apps/web/notes", `{"description":"a\u200bb"}`, http.StatusBadRequest},
		{"description with ESC", "/api/v1/apps/web/notes", `{"description":"a\u001b[2J"}`, http.StatusBadRequest},
		{"notes with tab and CRLF", "/api/v1/apps/web/notes", `{"notes":"a\tb\r\nc"}`, http.StatusOK},
		{"not json", "/api/v1/apps/web/notes", `nope`, http.StatusBadRequest},
		{"missing app", "/api/v1/apps/nope/notes", `{"notes":"x"}`, http.StatusNotFound},
		{"missing database", "/api/v1/databases/nope/notes", `{"notes":"x"}`, http.StatusNotFound},
		{"at cap", "/api/v1/apps/web/notes", `{"description":"ten chars!","notes":"` + strings.Repeat("y", 1024) + `"}`, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, tc.path, tc.body))
			if rec.Code != tc.want {
				t.Fatalf("got %d %s, want %d", rec.Code, rec.Body.String(), tc.want)
			}
		})
	}
}

func notesTarEntry(t *testing.T, archive []byte, name string) string {
	t.Helper()
	tr := tar.NewReader(bytes.NewReader(archive))
	for {
		h, err := tr.Next()
		if err != nil {
			t.Fatalf("%s not in export: %v", name, err)
		}
		if strings.HasSuffix(h.Name, name) {
			b, _ := io.ReadAll(tr)
			return string(b)
		}
	}
}

func TestGroupNotesIsStableAcrossMembers(t *testing.T) {
	stored := map[string]store.ResourceNotes{"shop-web": {Notes: "web notes"}, "shop-api": {Notes: "api notes"}}
	get := func(n string) store.ResourceNotes { return stored[n] }
	a := groupNotes([]string{"shop-web", "shop-api", "shop-db"}, get)
	b := groupNotes([]string{"shop-db", "shop-api", "shop-web"}, get)
	if a != b || a.Notes != "api notes" {
		t.Fatalf("group notes differ by member order: %+v %+v", a, b)
	}
	if got := groupNotes([]string{"shop-db"}, get); got != (store.ResourceNotes{}) {
		t.Fatalf("no notes = %+v", got)
	}
}
