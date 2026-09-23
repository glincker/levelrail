package spec

import (
	"strings"
	"testing"
)

func TestSpec_Validate(t *testing.T) {
	tests := []struct {
		name    string
		spec    *Spec
		wantErr string
	}{
		{
			name: "valid spec",
			spec: &Spec{
				Services: map[string]Service{
					"app": {
						Build: Build{
							Type:  BuildImage,
							Image: "nginx:latest",
						},
						Port:    80,
						Domains: []string{"example.com"},
					},
				},
				Databases: map[string]Database{
					"db": {
						Engine: EnginePostgres,
					},
				},
			},
		},
		{
			name: "invalid service name",
			spec: &Spec{
				Services: map[string]Service{
					"App": {},
				},
			},
			wantErr: "name must be lowercase alphanumeric and hyphens",
		},
		{
			name: "overlapping domains",
			spec: &Spec{
				Services: map[string]Service{
					"app-one": {
						Build:   Build{Type: BuildImage, Image: "nginx"},
						Port:    80,
						Domains: []string{"example.com"},
					},
					"app-two": {
						Build:   Build{Type: BuildImage, Image: "nginx"},
						Port:    80,
						Domains: []string{"example.com"},
					},
				},
			},
			wantErr: `domain "example.com" is claimed by both service`,
		},
		{
			name: "invalid database name",
			spec: &Spec{
				Services: map[string]Service{
					"app": {
						Build: Build{Type: BuildImage, Image: "nginx"},
						Port:  80,
					},
				},
				Databases: map[string]Database{
					"DB": {
						Engine: EnginePostgres,
					},
				},
			},
			wantErr: "name must be lowercase alphanumeric and hyphens",
		},
		{
			name: "unsupported database engine",
			spec: &Spec{
				Services: map[string]Service{
					"app": {
						Build: Build{Type: BuildImage, Image: "nginx"},
						Port:  80,
					},
				},
				Databases: map[string]Database{
					"db": {
						Engine: "sqlite",
					},
				},
			},
			wantErr: "is not supported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("Validate() error = %v, wantErr containing %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("Validate() unexpected error = %v", err)
			}
		})
	}
}
