package spec

import (
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
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
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
