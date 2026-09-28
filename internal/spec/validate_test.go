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
		{
			name: "valid dependsOn",
			spec: &Spec{
				Services: map[string]Service{
					"web": {
						Build:     Build{Type: BuildImage, Image: "nginx"},
						Port:      80,
						DependsOn: []string{"db"},
					},
					"db": {
						Build: Build{Type: BuildImage, Image: "postgres"},
						Port:  5432,
					},
				},
			},
		},
		{
			name: "dependsOn references itself",
			spec: &Spec{
				Services: map[string]Service{
					"web": {
						Build:     Build{Type: BuildImage, Image: "nginx"},
						Port:      80,
						DependsOn: []string{"web"},
					},
				},
			},
			wantErr: "dependsOn must not reference itself",
		},
		{
			name: "dependsOn references an unknown service",
			spec: &Spec{
				Services: map[string]Service{
					"web": {
						Build:     Build{Type: BuildImage, Image: "nginx"},
						Port:      80,
						DependsOn: []string{"db"},
					},
				},
			},
			wantErr: `dependsOn references "db", which is not a service in this file`,
		},
		{
			name: "dependsOn references a static service",
			spec: &Spec{
				Services: map[string]Service{
					"web": {
						Build:     Build{Type: BuildImage, Image: "nginx"},
						Port:      80,
						DependsOn: []string{"site"},
					},
					"site": {
						Build: Build{Type: BuildStatic},
					},
				},
			},
			wantErr: "has no single running container to depend on",
		},
		{
			name: "dependsOn cycle between two services",
			spec: &Spec{
				Services: map[string]Service{
					"web": {
						Build:     Build{Type: BuildImage, Image: "nginx"},
						Port:      80,
						DependsOn: []string{"worker"},
					},
					"worker": {
						Build:     Build{Type: BuildImage, Image: "nginx"},
						Port:      80,
						DependsOn: []string{"web"},
					},
				},
			},
			wantErr: "dependsOn cycle",
		},
		{
			name: "dependsOn self-cycle through a third service",
			spec: &Spec{
				Services: map[string]Service{
					"a": {
						Build:     Build{Type: BuildImage, Image: "nginx"},
						Port:      80,
						DependsOn: []string{"b"},
					},
					"b": {
						Build:     Build{Type: BuildImage, Image: "nginx"},
						Port:      80,
						DependsOn: []string{"c"},
					},
					"c": {
						Build:     Build{Type: BuildImage, Image: "nginx"},
						Port:      80,
						DependsOn: []string{"a"},
					},
				},
			},
			wantErr: "dependsOn cycle",
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

func TestService_EffectiveReplicas(t *testing.T) {
	tests := []struct {
		name     string
		replicas int
		want     int
	}{
		{"default fallback", 0, DefaultReplicas},
		{"explicit replicas", 5, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &Service{
				Replicas: tt.replicas,
			}
			if got := svc.EffectiveReplicas(); got != tt.want {
				t.Errorf("EffectiveReplicas() = %d, want %d", got, tt.want)
			}
		})
	}
}
