package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestBeginOnForgeCreatesAndMarksInProgress(t *testing.T) {
	fs, rt, rows, f := newDeploymentFixture(t, func(r *http.Request) (int, map[string]string, string) {
		if strings.HasSuffix(r.URL.Path, "/deployments") {
			return 201, nil, `{"id":77}`
		}
		return 201, nil, `{}`
	})
	d := rt.beginOnForge(context.Background(), f, "web", "sha1", forgeEnvPreview, previewScope(3), "https://web-pr-3.test")
	if d == nil {
		t.Fatal("deployment was not created")
	}
	if len(fs.calls) != 2 || fs.calls[0].Body["environment"] != "preview" || fs.calls[1].Body["state"] != "in_progress" {
		t.Fatalf("calls = %+v, want create then in_progress", fs.calls)
	}
	var stored bool
	for _, row := range rows.rows {
		stored = row.ExternalID == 77 && row.Environment == previewScope(3) && row.State == "in_progress"
	}
	if !stored {
		t.Fatalf("rows = %+v", rows.rows)
	}
}

func TestBeginOnForgeRecordsARefusedDeployment(t *testing.T) {
	tests := []struct {
		name   string
		status int
		header map[string]string
		want   string
	}{
		{"permission missing", 403, nil, "deployment not created"},
		{"rate limited", 429, map[string]string{"Retry-After": "9"}, "retry after 9s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, rt, rows, f := newDeploymentFixture(t, func(*http.Request) (int, map[string]string, string) {
				return tt.status, tt.header, "no"
			})
			if d := rt.beginOnForge(context.Background(), f, "web", "sha1", forgeEnvProduction, forgeEnvProduction, ""); d != nil {
				t.Fatal("a refused deployment returned a handle")
			}
			if len(rows.rows) != 1 {
				t.Fatalf("rows = %+v, want one error row", rows.rows)
			}
			for _, row := range rows.rows {
				if row.State != "error" || !strings.Contains(row.Warning, tt.want) {
					t.Fatalf("row = %+v, want an error row containing %q", row, tt.want)
				}
			}
		})
	}
}
