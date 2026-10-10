package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestRunVersionJSONReportsSchema(t *testing.T) {
	var out bytes.Buffer
	if err := runVersion([]string{"--json"}, &out); err != nil {
		t.Fatal(err)
	}
	var got versionInfo
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode %q: %v", out.String(), err)
	}
	want, err := store.MaxSchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != want || got.Version == "" {
		t.Fatalf("got %+v, want schema %d", got, want)
	}
}

func TestParseRollbackFlags(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"valid", []string{"--to", "v1.2.3", "--dry-run"}, ""},
		{"missing target", []string{"--dry-run"}, "invalid flags"},
		{"path traversal rejected", []string{"--to", "../../etc/passwd"}, "not a release version"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseRollbackFlags(tc.args, &bytes.Buffer{})
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}
