package spec

import (
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestEnvVar_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want EnvVar
	}{
		{
			name: "plain string is a literal value",
			yaml: `FOO: bar`,
			want: EnvVar{Value: "bar"},
		},
		{
			name: "from reference",
			yaml: `FOO: { from: postgres.main.url }`,
			want: EnvVar{From: "postgres.main.url"},
		},
		{
			name: "secret and required",
			yaml: `FOO: { secret: true, required: true }`,
			want: EnvVar{Secret: true, Required: true},
		},
		{
			name: "vault reference",
			yaml: `FOO: { vault: { path: secret/data/myapp, key: api_key } }`,
			want: EnvVar{Vault: &VaultRef{Path: "secret/data/myapp", Key: "api_key"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m map[string]EnvVar
			if err := yaml.Unmarshal([]byte(tt.yaml), &m); err != nil {
				t.Fatalf("yaml.Unmarshal() error = %v", err)
			}
			got, ok := m["FOO"]
			if !ok {
				t.Fatal("expected key FOO")
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestEnvVar_UnmarshalYAML_VaultMutualExclusion(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "vault and secret",
			yaml: `FOO: { vault: { path: p, key: k }, secret: true }`,
		},
		{
			name: "vault and from",
			yaml: `FOO: { vault: { path: p, key: k }, from: postgres.main.url }`,
		},
		{
			name: "vault missing key",
			yaml: `FOO: { vault: { path: p } }`,
		},
		{
			name: "vault missing path",
			yaml: `FOO: { vault: { key: k } }`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m map[string]EnvVar
			if err := yaml.Unmarshal([]byte(tt.yaml), &m); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestEnvVar_UnmarshalYAML_InvalidShape(t *testing.T) {
	var m map[string]EnvVar
	err := yaml.Unmarshal([]byte(`FOO: [1, 2, 3]`), &m)
	if err == nil {
		t.Fatal("expected an error for a sequence value, want string or object")
	}
}
