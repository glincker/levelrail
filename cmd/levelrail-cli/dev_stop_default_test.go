package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/localmode"
)

func devTestEnv(home string) func(string) (string, bool) {
	m := map[string]string{"HOME": home, "XDG_DATA_HOME": ""}
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

// A start with a custom dir leaves a pointer in the default dir, so a
// later stop or status without --data-dir still finds the instance.
func TestDevFollowsCustomDirFromDefault(t *testing.T) {
	home, custom := t.TempDir(), t.TempDir()
	env := devTestEnv(home)
	def, err := defaultDevDataDir("levelrail-cli", env)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := followCustomDevDir(def); ok {
		t.Fatal("no pointer yet, want none")
	}
	if err := localmode.WriteState(localmode.State{PID: os.Getpid(), DataDir: custom}); err != nil {
		t.Fatal(err)
	}
	if err := recordCustomDevDir(def, custom); err != nil {
		t.Fatal(err)
	}
	got, ok := followCustomDevDir(def)
	if !ok || got != custom {
		t.Fatalf("follow = %q, %v; want %q", got, ok, custom)
	}
	var out, errb bytes.Buffer
	if code := runDev("levelrail-cli", []string{"status", "--json"}, &out, &errb, env); code != exitOK || !strings.Contains(out.String(), custom) {
		t.Errorf("status code %d out %q err %q; want it to report %s", code, out.String(), errb.String(), custom)
	}
}

func TestDevStopNotRunningNamesFlag(t *testing.T) {
	var out, errb bytes.Buffer
	code := runDev("levelrail-cli", []string{"stop"}, &out, &errb, devTestEnv(t.TempDir()))
	if code != exitOK || !strings.Contains(out.String(), "--data-dir") {
		t.Errorf("code %d out %q; want not running with a --data-dir hint", code, out.String())
	}
}
