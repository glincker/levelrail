package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolveOutputFormat(t *testing.T) {
	tests := []struct {
		name       string
		jsonOut    bool
		outputFlag string
		want       outputFormat
		wantErr    bool
	}{
		{name: "default", want: outputTable},
		{name: "json bool", jsonOut: true, want: outputJSON},
		{name: "output json", outputFlag: "json", want: outputJSON},
		{name: "output table", outputFlag: "table", want: outputTable},
		{name: "output text", outputFlag: "text", want: outputText},
		{name: "json bool plus matching output json", jsonOut: true, outputFlag: "json", want: outputJSON},
		{name: "json bool conflicts with output table", jsonOut: true, outputFlag: "table", wantErr: true},
		{name: "json bool conflicts with output text", jsonOut: true, outputFlag: "text", wantErr: true},
		{name: "invalid output value", outputFlag: "xml", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveOutputFormat(tt.jsonOut, tt.outputFlag)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveOutputFormat(%v, %q) = %v, nil; want an error", tt.jsonOut, tt.outputFlag, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveOutputFormat(%v, %q) error = %v", tt.jsonOut, tt.outputFlag, err)
			}
			if got != tt.want {
				t.Errorf("resolveOutputFormat(%v, %q) = %q, want %q", tt.jsonOut, tt.outputFlag, got, tt.want)
			}
		})
	}
}

// TestApplyQuery_MultiItemList proves --query's projection against a real
// multi-item list response, the exact shape "apps list" returns: a bare
// JSON array of app objects, not wrapped in an "apps" key.
func TestApplyQuery_MultiItemList(t *testing.T) {
	apps := []appResource{
		{Name: "web", Image: "levelrail/web:3", Port: 3000, NodeID: "node-1"},
		{Name: "worker", Image: "levelrail/worker:1", Port: 4000, NodeID: "node-2"},
		{Name: "api", Image: "levelrail/api:7", Port: 5000, NodeID: "node-1"},
	}

	t.Run("filter by field, project one column", func(t *testing.T) {
		got, err := applyQuery("[?node_id=='node-1'].name", apps)
		if err != nil {
			t.Fatalf("applyQuery() error = %v", err)
		}
		want := []any{"web", "api"}
		gotSlice, ok := got.([]any)
		if !ok || len(gotSlice) != len(want) {
			t.Fatalf("applyQuery() = %#v, want %#v", got, want)
		}
		for i, w := range want {
			if gotSlice[i] != w {
				t.Errorf("applyQuery()[%d] = %v, want %v", i, gotSlice[i], w)
			}
		}
	})

	t.Run("index into the list", func(t *testing.T) {
		got, err := applyQuery("[0].image", apps)
		if err != nil {
			t.Fatalf("applyQuery() error = %v", err)
		}
		if got != "levelrail/web:3" {
			t.Errorf("applyQuery([0].image) = %v, want %q", got, "levelrail/web:3")
		}
	})

	t.Run("length aggregate", func(t *testing.T) {
		got, err := applyQuery("length(@)", apps)
		if err != nil {
			t.Fatalf("applyQuery() error = %v", err)
		}
		if got != float64(3) {
			t.Errorf("applyQuery(length(@)) = %v, want 3", got)
		}
	})

	t.Run("invalid expression is a validation error", func(t *testing.T) {
		_, err := applyQuery("[?bad syntax", apps)
		if err == nil {
			t.Fatal("applyQuery() error = nil, want a syntax error")
		}
		if exitCodeForError(err) != exitValidation {
			t.Errorf("exitCodeForError(%v) = %d, want %d (a bad --query is a usage mistake, not a network failure)", err, exitCodeForError(err), exitValidation)
		}
	})
}

// TestWriteTextValue_DiffersFromTable proves --output text produces a
// visibly different shape than --output table for the same data: no
// header, no column alignment, tab-separated, script-friendly.
func TestWriteTextValue_DiffersFromTable(t *testing.T) {
	apps := []appResource{
		{Name: "web", Image: "levelrail/web:3", Port: 3000},
		{Name: "worker", Image: "levelrail/worker:1", Port: 4000, NodeID: "node-1"},
	}

	var tableBuf bytes.Buffer
	printAppsTable(&tableBuf, apps)
	tableOut := tableBuf.String()

	generic, err := toGenericJSON(apps)
	if err != nil {
		t.Fatalf("toGenericJSON() error = %v", err)
	}
	var textBuf bytes.Buffer
	if err := writeTextValue(&textBuf, generic); err != nil {
		t.Fatalf("writeTextValue() error = %v", err)
	}
	textOut := textBuf.String()

	if tableOut == textOut {
		t.Fatalf("table and text output must differ; both were:\n%s", tableOut)
	}
	if strings.Contains(textOut, "NAME") || strings.Contains(textOut, "IMAGE") {
		t.Errorf("text output = %q, must not include table headers", textOut)
	}
	if !strings.Contains(tableOut, "NAME") {
		t.Errorf("table output = %q, want a NAME header", tableOut)
	}
	// Text is tab-separated with no box-drawing/alignment padding: each
	// data line should contain literal tabs, one field per column.
	lines := strings.Split(strings.TrimRight(textOut, "\n"), "\n")
	if len(lines) != len(apps) {
		t.Fatalf("text output has %d lines, want %d (one per app)", len(lines), len(apps))
	}
	for _, line := range lines {
		if !strings.Contains(line, "\t") {
			t.Errorf("text line %q has no tab separator", line)
		}
	}
}

func TestWriteQueriedTable_FallsBackToJSONForNonTabularShape(t *testing.T) {
	var buf bytes.Buffer
	// A scalar (as a query like "[0].image" produces) has no rows/columns
	// to lay out as a table, so it must fall back to JSON rather than
	// guess a layout.
	if err := writeQueriedTable(&buf, "levelrail/web:3"); err != nil {
		t.Fatalf("writeQueriedTable() error = %v", err)
	}
	var got string
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("fallback output is not valid JSON: %v; got: %s", err, buf.String())
	}
	if got != "levelrail/web:3" {
		t.Errorf("fallback JSON = %q, want %q", got, "levelrail/web:3")
	}
}

func TestWriteQueriedTable_UniformObjectList(t *testing.T) {
	apps := []appResource{
		{Name: "web", Image: "levelrail/web:3", Port: 3000},
		{Name: "worker", Image: "levelrail/worker:1", Port: 4000},
	}
	projected, err := applyQuery("[].{name: name, image: image}", apps)
	if err != nil {
		t.Fatalf("applyQuery() error = %v", err)
	}
	var buf bytes.Buffer
	if err := writeQueriedTable(&buf, projected); err != nil {
		t.Fatalf("writeQueriedTable() error = %v", err)
	}
	out := buf.String()
	for _, want := range []string{"NAME", "IMAGE", "web", "levelrail/web:3", "worker", "levelrail/worker:1"} {
		if !strings.Contains(out, want) {
			t.Errorf("queried table output missing %q; got:\n%s", want, out)
		}
	}
}

// TestRun_AppsList_Query is an end-to-end regression test through the
// same "apps list" command the getting-started docs use, proving --query
// filters a real multi-item HTTP response exactly the way an AWS CLI
// user would expect (no "apps" wrapper key: ListApps' wire response is a
// bare JSON array, so the expression indexes straight into it).
func TestRun_AppsList_Query(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]appResource{
			{Name: "web", Image: "levelrail/web:3", Port: 3000, NodeID: "node-1"},
			{Name: "worker", Image: "levelrail/worker:1", Port: 4000, NodeID: "node-2"},
			{Name: "api", Image: "levelrail/api:7", Port: 5000, NodeID: "node-1"},
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "list", "--query", "[?node_id=='node-1'].name", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	var names []string
	if err := json.Unmarshal(stdout.Bytes(), &names); err != nil {
		t.Fatalf("stdout is not valid JSON: %v; got: %s", err, stdout.String())
	}
	if len(names) != 2 || names[0] != "web" || names[1] != "api" {
		t.Errorf("names = %v, want [web api]", names)
	}
}

// TestRun_AppsList_OutputText proves --output text renders a visibly
// different, script-friendly shape from the default --output table for
// the exact same "apps list" response.
func TestRun_AppsList_OutputText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]appResource{
			{Name: "web", Image: "levelrail/web:3", Port: 3000, NodeID: "node-1"},
		})
	}))
	defer srv.Close()

	var tableOut, textOut, stderr bytes.Buffer
	if got := run("levelrail-cli-test", []string{"apps", "list", "--api-url", srv.URL}, &tableOut, &stderr, envMap()); got != exitOK {
		t.Fatalf("table exit = %d, want %d (stderr=%q)", got, exitOK, stderr.String())
	}
	stderr.Reset()
	if got := run("levelrail-cli-test", []string{"apps", "list", "--output", "text", "--api-url", srv.URL}, &textOut, &stderr, envMap()); got != exitOK {
		t.Fatalf("text exit = %d, want %d (stderr=%q)", got, exitOK, stderr.String())
	}

	if tableOut.String() == textOut.String() {
		t.Fatalf("--output table and --output text must differ; both were:\n%s", tableOut.String())
	}
	if !strings.Contains(tableOut.String(), "NAME") {
		t.Errorf("table output = %q, want a NAME header", tableOut.String())
	}
	if strings.Contains(textOut.String(), "NAME") {
		t.Errorf("text output = %q, must not include the table header", textOut.String())
	}
	// Fields flatten in sorted JSON-key order (env_dirty, image, name,
	// node_id, port), not struct-definition order: deterministic without
	// depending on the caller's field layout.
	if !strings.Contains(textOut.String(), "false\tlevelrail/web:3\tweb\tnode-1\t3000") {
		t.Errorf("text output = %q, want tab-separated fields in sorted-key order", textOut.String())
	}
}

// TestRun_AppsList_JSONBackwardCompat is a regression test proving
// --json still behaves exactly as it did before --output/--query
// existed: a bare JSON array on stdout, nothing else, same as always.
// Existing scripts built against --json must keep working unchanged.
func TestRun_AppsList_JSONBackwardCompat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]appResource{
			{Name: "web", Image: "levelrail/web:3", Port: 3000},
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "list", "--json", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitOK, stderr.String())
	}
	var apps []appResource
	if err := json.Unmarshal(stdout.Bytes(), &apps); err != nil {
		t.Fatalf("--json stdout is not a plain JSON array: %v; got: %s", err, stdout.String())
	}
	if len(apps) != 1 || apps[0].Name != "web" {
		t.Errorf("apps = %+v, want one app named web", apps)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty on a successful --json call", stderr.String())
	}

	// --output json must be indistinguishable from --json for the exact
	// same request: the whole point of the alias.
	var stdout2, stderr2 bytes.Buffer
	got2 := run("levelrail-cli-test", []string{"apps", "list", "--output", "json", "--api-url", srv.URL}, &stdout2, &stderr2, envMap())
	if got2 != exitOK {
		t.Fatalf("--output json exit = %d, want %d (stderr=%q)", got2, exitOK, stderr2.String())
	}
	if stdout.String() != stdout2.String() {
		t.Errorf("--json stdout = %q, --output json stdout = %q, want them identical", stdout.String(), stdout2.String())
	}
}

// TestRun_AppsList_JSONConflictsWithOutput proves passing both --json and
// a contradicting --output is a usage error, not a silent pick between
// the two.
func TestRun_AppsList_JSONConflictsWithOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "list", "--json", "--output", "table"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--json conflicts with --output") {
		t.Errorf("stderr = %q, want a --json/--output conflict error", stderr.String())
	}
}
