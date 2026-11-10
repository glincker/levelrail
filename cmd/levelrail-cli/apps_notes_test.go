package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsSet_NotesFromStdin(t *testing.T) {
	var gotPath string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"description": gotBody["description"], "notes": gotBody["notes"]})
	}))
	defer srv.Close()
	prev := notesStdin
	notesStdin = strings.NewReader("# Runbook\nowner: payments\n")
	defer func() { notesStdin = prev }()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "set", "web", "--description", "Checkout", "--notes-file", "-", "--api-url", srv.URL})
	if gotPath != "/api/v1/apps/web/notes" || gotBody["description"] != "Checkout" || !strings.Contains(gotBody["notes"], "owner: payments") {
		t.Fatalf("request %s %v", gotPath, gotBody)
	}
	if !strings.Contains(stdout, "description: Checkout") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRun_AppsGet_IncludesNotesInJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/notes") {
			_, _ = w.Write([]byte(`{"description":"Checkout","notes":"owner: payments"}`))
			return
		}
		_, _ = w.Write([]byte(`{"name":"web","image":"nginx:1","port":80}`))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "get", "web", "--output", "json", "--api-url", srv.URL})
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("json: %v %q", err, stdout)
	}
	if got["name"] != "web" || got["description"] != "Checkout" || got["notes"] != "owner: payments" {
		t.Fatalf("got %v", got)
	}
}

func TestPrintAppNotesStripsTerminalEscapes(t *testing.T) {
	var out strings.Builder
	printAppNotes(&out, "ok\u202eevil", "line one\x1b]52;c;ZXZpbA==\x07\n\x1b[2Jline two\u200b\ttabbed")
	got := out.String()
	for _, bad := range []string{"\x1b", "\x07", "\u202e", "\u200b", "\t"} {
		if strings.Contains(got, bad) {
			t.Fatalf("output kept %q: %q", bad, got)
		}
	}
	if !strings.Contains(got, "line one") || !strings.Contains(got, "\n  ") {
		t.Fatalf("output lost text or line breaks: %q", got)
	}
}
