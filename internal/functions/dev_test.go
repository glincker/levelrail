package functions

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDetectDevSpec(t *testing.T) {
	tests := []struct {
		file, runtime string
	}{
		{"package.json", "node"},
		{"requirements.txt", "python"},
		{"go.mod", "go"},
	}
	for _, tt := range tests {
		t.Run(tt.runtime, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, tt.file), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
			spec, err := DetectDevSpec(dir, "")
			if err != nil || spec.Runtime != tt.runtime || spec.Image == "" {
				t.Fatalf("spec = %+v err = %v", spec, err)
			}
		})
	}
	if _, err := DetectDevSpec(t.TempDir(), ""); err == nil {
		t.Error("an empty directory must not match a runtime")
	}
}

func TestShellCommandChangesIntoMount(t *testing.T) {
	got := DevSpec{Command: "go run ."}.ShellCommand()
	if len(got) != 3 || got[2] != "cd /app && go run ." {
		t.Errorf("command = %v", got)
	}
}

func TestParseEnvFile(t *testing.T) {
	env := ParseEnvFile([]byte("# c\nA=1\n\nB = \"two\"\nbad\nC=x=y\n"))
	if len(env) != 3 || env["A"] != "1" || env["B"] != "two" || env["C"] != "x=y" {
		t.Errorf("env = %v", env)
	}
}

func TestLimitsFromEnvOverridesAndFallsBack(t *testing.T) {
	env := map[string]string{"FUNCTION_INPUT_MAX_ENTRIES": "7", "FUNCTION_INPUT_RETENTION": "bad"}
	lim := InvokeLimitsFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if lim.InputEntries != 7 || lim.InputRetention != 24*time.Hour {
		t.Errorf("limits = %+v", lim)
	}
}
