package main

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

func managedEnv(t *testing.T, method string) (func(string) (string, bool), *[]string) {
	t.Helper()
	env, _ := updateTestEnv(t, "v0.2.0", map[string]string{"LEVELRAIL_CLI_INSTALL_METHOD": method})
	exe := t.TempDir() + "/levelrail-cli"
	oldExe, oldRun, oldCI := updateExePath, updateRunUpgrade, ciMode
	var ran []string
	updateExePath = func() (string, error) { return exe, nil }
	updateRunUpgrade = func(_ context.Context, argv []string, _, _ io.Writer) error {
		ran = argv
		return nil
	}
	ciMode = true
	t.Cleanup(func() { updateExePath, updateRunUpgrade, ciMode = oldExe, oldRun, oldCI })
	return env, &ran
}

func TestUpdateManagedPrintsCommandWithoutYes(t *testing.T) {
	cases := map[string]string{
		"npm":  "npm install --global levelrail-cli@latest",
		"pnpm": "pnpm add --global levelrail-cli@latest",
		"yarn": "yarn global add levelrail-cli@latest",
	}
	for method, want := range cases {
		t.Run(method, func(t *testing.T) {
			env, ran := managedEnv(t, method)
			var out, errb bytes.Buffer
			if code := runUpdate("levelrail-cli", nil, &out, &errb, env); code != exitOK {
				t.Fatalf("code=%d stderr=%s", code, errb.String())
			}
			if !strings.Contains(out.String(), want) || len(*ran) != 0 {
				t.Fatalf("out=%s ran=%v", out.String(), *ran)
			}
		})
	}
}

func TestUpdateManagedYesRunsPackageManager(t *testing.T) {
	env, ran := managedEnv(t, "npm")
	var out, errb bytes.Buffer
	if code := runUpdate("levelrail-cli", []string{"--yes"}, &out, &errb, env); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, errb.String())
	}
	if strings.Join(*ran, " ") != "npm install --global levelrail-cli@latest" {
		t.Fatalf("ran=%v", *ran)
	}
}

func TestUpdateManagedDryRunAndNPX(t *testing.T) {
	env, ran := managedEnv(t, "npm")
	var out, errb bytes.Buffer
	if code := runUpdate("levelrail-cli", []string{"--dry-run", "--yes"}, &out, &errb, env); code != exitOK || len(*ran) != 0 {
		t.Fatalf("code=%d ran=%v", code, *ran)
	}
	env, ran = managedEnv(t, "npx")
	out.Reset()
	if code := runUpdate("levelrail-cli", []string{"--yes"}, &out, &errb, env); code != exitOK || len(*ran) != 0 || !strings.Contains(out.String(), "npx levelrail-cli@latest") {
		t.Fatalf("code=%d out=%s ran=%v", code, out.String(), *ran)
	}
}
