package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestEnvironmentsSetDefaultHelp(t *testing.T) {
	for _, run := range []func(string, []string, *bytes.Buffer, *bytes.Buffer) int{
		func(p string, a []string, o, e *bytes.Buffer) int {
			return runEnvironments(p, a, o, e, func(string) (string, bool) { return "", false })
		},
		func(p string, a []string, o, e *bytes.Buffer) int {
			return runAppsEnvironments(p, a, o, e, func(string) (string, bool) { return "", false })
		},
	} {
		var out, errb bytes.Buffer
		run("levelrail-cli", []string{"help"}, &out, &errb)
		if !strings.Contains(out.String(), "set-default") {
			t.Errorf("usage does not mention set-default: %s", out.String())
		}
	}
	var out, errb bytes.Buffer
	if code := runAppsProjects("levelrail-cli", []string{"backfill-default-env", "-h"}, &out, &errb, func(string) (string, bool) { return "", false }); code != exitOK && code != exitUsage {
		t.Errorf("backfill help exit = %d", code)
	}
	if !strings.Contains(errb.String()+out.String(), "--yes") {
		t.Errorf("backfill usage lacks --yes")
	}
}
