package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestTerminalURL(t *testing.T) {
	tests := []struct {
		name    string
		apiURL  string
		app     string
		command []string
		want    string
		wantErr bool
	}{
		{
			name:   "http becomes ws",
			apiURL: "http://localhost:8080",
			app:    "web",
			want:   "ws://localhost:8080/api/v1/apps/web/terminal?cols=80&rows=24",
		},
		{
			name:   "https becomes wss",
			apiURL: "https://levelrail.example.com/",
			app:    "web",
			want:   "wss://levelrail.example.com/api/v1/apps/web/terminal?cols=80&rows=24",
		},
		{
			name:    "explicit command becomes repeated query params",
			apiURL:  "http://localhost:8080",
			app:     "web",
			command: []string{"/bin/bash", "-l"},
			want:    "ws://localhost:8080/api/v1/apps/web/terminal?cols=80&command=%2Fbin%2Fbash&command=-l&rows=24",
		},
		{
			name:   "app name is escaped",
			apiURL: "http://localhost:8080",
			app:    "web/prod",
			want:   "ws://localhost:8080/api/v1/apps/web%2Fprod/terminal?cols=80&rows=24",
		},
		{
			name:    "a non-http scheme is rejected",
			apiURL:  "ftp://localhost:8080",
			app:     "web",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := terminalURL(tt.apiURL, tt.app, tt.command, 24, 80)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("terminalURL() = %q, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("terminalURL() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("terminalURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRunAppsExecInteractive_RequiresAppName proves the interactive path
// keeps the app name required even though the command is optional.
func TestRunAppsExecInteractive_RequiresAppName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runAppsExecInteractiveFromFlags(nil, "levelrail", credentialFlags{}, func(string) (string, bool) { return "", false }, &stdout, &stderr)
	if code != exitUsage {
		t.Errorf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires an app name") {
		t.Errorf("stderr = %q, want it to name the missing app", stderr.String())
	}
}

// TestRunAppsExecInteractive_RequiresATerminal proves a piped stdin gets
// a clear refusal rather than a session that can never work. The test
// process's own stdin is never a terminal under `go test`.
func TestRunAppsExecInteractive_RequiresATerminal(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runAppsExecInteractive("levelrail", "web", nil, terminalCredentials{APIURL: "http://localhost:8080"}, &stdout, &stderr)
	if code != exitUsage {
		t.Errorf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr.String(), "needs a terminal on stdin") {
		t.Errorf("stderr = %q, want it to explain the missing terminal", stderr.String())
	}
}
