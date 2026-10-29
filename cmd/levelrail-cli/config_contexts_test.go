package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestConfigContextsRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := writeCredentialsFile("lrtest", "prod", credentials{APIURL: "https://prod.example", Token: "x"}); err != nil {
		t.Fatal(err)
	}
	env := func(string) (string, bool) { return "", false }

	var out, errb bytes.Buffer
	if code := runConfig("lrtest", []string{"use-context", "nope"}, &out, &errb, env); code == exitOK {
		t.Fatal("unknown context should fail")
	}
	out.Reset()
	if code := runConfig("lrtest", []string{"use-context", "prod"}, &out, &errb, env); code != exitOK {
		t.Fatalf("use-context code %d: %s", code, errb.String())
	}
	if readCurrentContext("lrtest") != "prod" {
		t.Fatal("current context not saved")
	}
	lookup := currentContextLookup("lrtest", env)
	if v, _ := lookup(envProfile); v != "prod" {
		t.Errorf("lookup gave %q", v)
	}
	out.Reset()
	if code := runConfig("lrtest", []string{"get-contexts"}, &out, &errb, lookup); code != exitOK || !strings.Contains(out.String(), "*        prod") {
		t.Errorf("get-contexts code %d:\n%s", code, out.String())
	}
	out.Reset()
	_ = runConfig("lrtest", []string{"current-context"}, &out, &errb, lookup)
	if strings.TrimSpace(out.String()) != "prod" {
		t.Errorf("current-context %q", out.String())
	}
	_ = os.Remove(home)
}
