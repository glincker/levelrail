package main

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
)

func TestPlanFromFile_RequiredSecrets(t *testing.T) {
	fileSpec := func(env map[string]spec.EnvVar) *spec.Spec {
		return &spec.Spec{Version: 1, Services: map[string]spec.Service{
			"web": {Build: spec.Build{Type: spec.BuildImage, Image: "shop/web:1"}, Port: 3000, Env: env},
		}}
	}
	tests := []struct {
		name    string
		env     map[string]spec.EnvVar
		flags   createFlags
		wantErr string
	}{
		{
			name:    "required secret without a value",
			env:     map[string]spec.EnvVar{"API_KEY": {Secret: true, Required: true}, "B_KEY": {Secret: true, Required: true}},
			wantErr: "API_KEY, B_KEY",
		},
		{
			name:  "required secret given with --secret",
			env:   map[string]spec.EnvVar{"API_KEY": {Secret: true, Required: true}},
			flags: createFlags{secrets: map[string]string{"API_KEY": "v"}},
		},
		{
			name:  "required secret given with --vault-secret",
			env:   map[string]spec.EnvVar{"API_KEY": {Secret: true, Required: true}},
			flags: createFlags{vaultSecrets: map[string]appVaultEnvRef{"API_KEY": {Path: "kv/app", Key: "k"}}},
		},
		{
			name: "optional secret without a value",
			env:  map[string]spec.EnvVar{"API_KEY": {Secret: true}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.flags
			f.file = "app.yaml"
			_, err := planFromFile(f, fileSpec(tt.env), detectedGit{})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("planFromFile() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), "--secret") {
				t.Fatalf("planFromFile() error = %v, want one naming %q and --secret", err, tt.wantErr)
			}
		})
	}
}
