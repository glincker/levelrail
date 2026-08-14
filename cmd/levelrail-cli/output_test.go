package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestWriteJSONValue(t *testing.T) {
	var buf bytes.Buffer
	app := appResource{Name: "web", Image: "levelrail/web:1", Port: 3000}
	if err := writeJSONValue(&buf, app); err != nil {
		t.Fatalf("writeJSONValue() error = %v", err)
	}

	var got appResource
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v; got: %s", err, buf.String())
	}
	if !reflect.DeepEqual(got, app) {
		t.Errorf("decoded = %+v, want %+v", got, app)
	}
}

func TestWriteJSONError(t *testing.T) {
	var buf bytes.Buffer
	if err := writeJSONError(&buf, errors.New("boom")); err != nil {
		t.Fatalf("writeJSONError() error = %v", err)
	}

	var got map[string]string
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v; got: %s", err, buf.String())
	}
	if got["error"] != "boom" {
		t.Errorf("error field = %q, want %q", got["error"], "boom")
	}
}

func TestExitCodeForError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "api error", err: &apiError{StatusCode: 400, Message: "bad"}, want: exitAPIError},
		{name: "wrapped api error", err: fmt.Errorf("create app: %w", &apiError{StatusCode: 404, Message: "not found"}), want: exitAPIError},
		{name: "validation error", err: newValidationError("missing --name"), want: exitValidation},
		{name: "wrapped validation error", err: fmt.Errorf("plan: %w", newValidationError("missing --name")), want: exitValidation},
		{name: "network error", err: errors.New("connection refused"), want: exitNetwork},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := exitCodeForError(tt.err); got != tt.want {
				t.Errorf("exitCodeForError(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

func TestPrintAppsTable(t *testing.T) {
	var buf bytes.Buffer
	printAppsTable(&buf, nil)
	if !strings.Contains(buf.String(), "no apps") {
		t.Errorf("empty list output = %q, want it to mention \"no apps\"", buf.String())
	}

	buf.Reset()
	printAppsTable(&buf, []appResource{
		{Name: "web", Image: "levelrail/web:1", Port: 3000},
		{Name: "worker", Image: "levelrail/worker:1", Port: 4000, NodeID: "node-1"},
	})
	out := buf.String()
	for _, want := range []string{"NAME", "web", "levelrail/web:1", "3000", "(local)", "worker", "node-1"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q; got:\n%s", want, out)
		}
	}
}
