package deploy

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/spec"
)

var testPrefixes = []string{"NEXT_PUBLIC_", "VITE_", "PUBLIC_", "REACT_APP_", "NUXT_PUBLIC_", "GATSBY_", "EXPO_PUBLIC_"}

func testEnv() map[string]spec.EnvVar {
	return map[string]spec.EnvVar{
		"NEXT_PUBLIC_API":       {Value: "https://api.example.com"},
		"VITE_X":                {Value: "1"},
		"REACT_APP_Y":           {Value: "2"},
		"NUXT_PUBLIC_Z":         {Value: "3"},
		"EXPO_PUBLIC_W":         {Value: "4"},
		"NEXT_PUBLIC_SECRETISH": {Value: "hidden", Secret: true},
		"NEXT_PUBLIC_REF":       {From: "other"},
		"DATABASE_URL":          {Value: "postgres://x"},
		"CRON_SECRET":           {Value: "s3cret"},
	}
}

func TestPublicBuildEnvClassification(t *testing.T) {
	got := staticBuildArgs(testEnv(), nil, testPrefixes)
	want := map[string]string{"NEXT_PUBLIC_API": "https://api.example.com", "VITE_X": "1", "REACT_APP_Y": "2", "NUXT_PUBLIC_Z": "3", "EXPO_PUBLIC_W": "4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestDockerfileBuildArgsOnlyDeclaredARGs(t *testing.T) {
	dir := t.TempDir()
	df := filepath.Join(dir, "Dockerfile")
	if err := os.WriteFile(df, []byte("FROM node\nARG NEXT_PUBLIC_API\narg VITE_X=dflt\nRUN true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := dockerfileBuildArgs(testEnv(), map[string]string{"EXPLICIT": "e", "VITE_X": "override"}, df, testPrefixes)
	want := map[string]string{"NEXT_PUBLIC_API": "https://api.example.com", "VITE_X": "override", "EXPLICIT": "e"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	if got := dockerfileBuildArgs(testEnv(), nil, filepath.Join(dir, "missing"), testPrefixes); len(got) != 0 {
		t.Fatalf("missing Dockerfile passed %v", got)
	}
}

func TestEmitBuildEnvKeysNamesKeysNotValues(t *testing.T) {
	var logs []string
	emitBuildEnvKeys(func(e build.ProgressEvent) { logs = append(logs, e.Log) }, "railpack", map[string]string{"NEXT_PUBLIC_B": "valueB", "NEXT_PUBLIC_A": "valueA"}, 2)
	line := strings.Join(logs, "")
	if !strings.Contains(line, "NEXT_PUBLIC_A, NEXT_PUBLIC_B") || strings.Contains(line, "value") {
		t.Fatalf("line = %q", line)
	}
	logs = nil
	emitBuildEnvKeys(func(e build.ProgressEvent) { logs = append(logs, e.Log) }, "railpack", nil, 3)
	if len(logs) != 1 || !strings.Contains(logs[0], "none passed") {
		t.Fatalf("empty line = %v", logs)
	}
}

func TestEmitBuildEnvKeysSilentWithoutAppEnv(t *testing.T) {
	emitBuildEnvKeys(func(e build.ProgressEvent) { t.Fatalf("unexpected event %+v", e) }, "railpack", nil, 0)
}
