package main

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
)

func TestCheckUnappliedSections(t *testing.T) {
	db := map[string]spec.Database{"main": {Engine: "postgres"}}
	envFrom := map[string]spec.Service{"web": {Env: map[string]spec.EnvVar{"DB_URL": {From: "databases.main.url"}, "PLAIN": {Value: "x"}}}}

	tests := []struct {
		name    string
		spec    *spec.Spec
		wantErr bool
		want    []string
		notWant []string
	}{
		{name: "neither", spec: &spec.Spec{Services: map[string]spec.Service{"web": {Env: map[string]spec.EnvVar{"PLAIN": {Value: "x"}}}}}},
		{name: "databases", spec: &spec.Spec{Databases: db}, wantErr: true, want: []string{"databases.main (postgres)", "import config"}, notWant: []string{"env."}},
		{name: "env from", spec: &spec.Spec{Services: envFrom}, wantErr: true, want: []string{"services.web.env.DB_URL (from: databases.main.url)", "import config"}, notWant: []string{"databases.main (", "PLAIN"}},
		{name: "both", spec: &spec.Spec{Databases: db, Services: envFrom}, wantErr: true, want: []string{"databases.main (postgres)", "services.web.env.DB_URL", "import config"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkUnappliedSections(tt.spec, "web", "levelrail", "app.yaml")
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil {
				return
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q missing %q", err, w)
				}
			}
			for _, w := range tt.notWant {
				if strings.Contains(err.Error(), w) {
					t.Errorf("error %q should not contain %q", err, w)
				}
			}
		})
	}
}
