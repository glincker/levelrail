package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestUnknownCommand_SuggestsClosestVerb(t *testing.T) {
	var out, errb bytes.Buffer
	code := run("levelrail-cli", []string{"depoy"}, &out, &errb, func(string) (string, bool) { return "", false })
	if code != exitUsage {
		t.Fatalf("exit = %d", code)
	}
	for _, want := range []string{"unknown command", "did you mean: levelrail-cli deploy", "--help"} {
		if !strings.Contains(errb.String(), want) {
			t.Errorf("stderr missing %q: %q", want, errb.String())
		}
	}
}

func TestUnknownFlag_SuggestsClosestFlag(t *testing.T) {
	var out, errb bytes.Buffer
	code := run("levelrail-cli", []string{"apps", "create", "--nmae", "x"}, &out, &errb, func(string) (string, bool) { return "", false })
	if code == exitOK {
		t.Fatalf("want failure")
	}
	if !strings.Contains(errb.String(), "did you mean: --name") {
		t.Errorf("stderr missing flag suggestion: %q", errb.String())
	}
}

func TestEmptyHint_NamesNextStep(t *testing.T) {
	var b bytes.Buffer
	printAppsTable(&b, nil)
	if !strings.Contains(b.String(), "no apps yet: run") || !strings.Contains(b.String(), "apps create") {
		t.Errorf("empty hint = %q", b.String())
	}
}
