package projecttpl

import (
	"errors"
	"testing"
)

func TestRenderEveryTemplateWithDefaults(t *testing.T) {
	needs := map[string]map[string]string{
		"cron-job": {"command": "node jobs/nightly.js"},
	}
	for _, info := range List() {
		vals := map[string]string{"name": "demo"}
		for k, v := range needs[info.ID] {
			vals[k] = v
		}
		files, err := Render(info.ID, vals)
		if err != nil {
			t.Fatalf("%s: %v", info.ID, err)
		}
		if len(files) == 0 || files[0].Path != "app.yaml" {
			t.Fatalf("%s: want app.yaml first, got %+v", info.ID, files)
		}
	}
}

func TestRenderValidation(t *testing.T) {
	tests := []struct {
		name string
		id   string
		vals map[string]string
		want string
	}{
		{"missing name", "web-service", map[string]string{}, "name"},
		{"bad name", "web-service", map[string]string{"name": "Bad_Name"}, "name"},
		{"bad port", "web-service", map[string]string{"name": "a", "port": "99999"}, "port"},
		{"bad enum", "web-service", map[string]string{"name": "a", "build": "nixpacks"}, "build"},
		{"bad cron", "cron-job", map[string]string{"name": "a", "command": "x", "schedule": "nope"}, "schedule"},
		{"unknown param", "worker", map[string]string{"name": "a", "bogus": "1"}, "bogus"},
		{"newline injection", "web-service", map[string]string{"name": "a", "health_path": "/x\nevil: 1"}, "health_path"},
		{"static path escape", "static-site", map[string]string{"name": "a", "path": "../x"}, "path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Render(tt.id, tt.vals)
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("want ValidationError, got %v", err)
			}
			found := false
			for _, f := range ve.Fields {
				found = found || f.Param == tt.want
			}
			if !found {
				t.Fatalf("want field %q in %+v", tt.want, ve.Fields)
			}
		})
	}
	if _, err := Render("nope", nil); !errors.Is(err, ErrUnknownTemplate) {
		t.Fatalf("want ErrUnknownTemplate, got %v", err)
	}
}

func TestQuotedValuesStayScalars(t *testing.T) {
	files, err := Render("build-test-deploy", map[string]string{"name": "a", "test_command": "echo \"hi\": #x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("want 2 files, got %d", len(files))
	}
}
