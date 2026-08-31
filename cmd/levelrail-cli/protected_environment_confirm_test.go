package main

import (
	"bytes"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// TestResolveProtectedEnvironmentConfirmation is a pure, table-driven
// test of the interactive prompt itself: no HTTP, no flag parsing,
// mirroring resolveRestoreConfirmation's own test shape
// (backups_restore_test.go).
func TestResolveProtectedEnvironmentConfirmation(t *testing.T) {
	tests := []struct {
		name    string
		stdin   string
		want    bool
		wantErr bool
	}{
		{name: "yes confirms", stdin: "yes\n", want: true},
		{name: "no refuses", stdin: "no\n", want: false},
		{name: "empty line refuses", stdin: "\n", want: false},
		{name: "garbage refuses", stdin: "sure\n", want: false},
		{name: "EOF refuses with an error, not a hang", stdin: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			got, err := resolveProtectedEnvironmentConfirmation("environment \"production\" is protected", strings.NewReader(tt.stdin), &stderr)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("confirmed = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProtectedEnvironmentError(t *testing.T) {
	conflict := &apiclient.APIError{StatusCode: http.StatusConflict, Message: "environment \"production\" is protected; set confirm: true to proceed"}
	if _, ok := protectedEnvironmentError(conflict); !ok {
		t.Errorf("protectedEnvironmentError(409) ok = false, want true")
	}

	notFound := &apiclient.APIError{StatusCode: http.StatusNotFound, Message: "app not found"}
	if _, ok := protectedEnvironmentError(notFound); ok {
		t.Errorf("protectedEnvironmentError(404) ok = true, want false")
	}

	if _, ok := protectedEnvironmentError(errors.New("boom")); ok {
		t.Errorf("protectedEnvironmentError(plain error) ok = true, want false")
	}

	if _, ok := protectedEnvironmentError(nil); ok {
		t.Errorf("protectedEnvironmentError(nil) ok = true, want false")
	}
}
