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

func TestProfileManageAndAllProfiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	env := func(string) (string, bool) { return "", false }
	var out, errb bytes.Buffer
	for _, args := range [][]string{
		{"set-context", "prod", "--api-url", "https://prod.example", "--team", "core"},
		{"set-context", "stage", "--api-url", "https://stage.example"},
		{"set-context", "stage", "--rename", "qa"},
	} {
		if code := runConfig("lrtest", args, &out, &errb, env); code != exitOK {
			t.Fatalf("%v code %d: %s", args, code, errb.String())
		}
	}
	if code := runProfile("lrtest", []string{"use", "qa"}, &out, &errb, env); code != exitOK || readCurrentContext("lrtest") != "qa" {
		t.Fatalf("use failed: %d %s", code, errb.String())
	}
	if code := runProfile("lrtest", []string{"remove", "qa"}, &out, &errb, env); code == exitOK {
		t.Fatal("remove without --yes must not proceed in a non-terminal")
	}
	if code := runProfile("lrtest", []string{"remove", "qa", "--yes"}, &out, &errb, env); code != exitOK {
		t.Fatalf("remove code %d: %s", code, errb.String())
	}
	names, _ := savedProfileNames("lrtest")
	if len(names) != 1 || names[0] != "prod" || readCurrentContext("lrtest") != "" {
		t.Fatalf("after remove: %v current %q", names, readCurrentContext("lrtest"))
	}
	res, err := runAcrossProfiles("lrtest", env, func(*Client) (any, error) { return nil, nil })
	if err != nil || len(res) != 1 || res[0].Err == nil {
		t.Fatalf("unsigned profile must fail softly: %v %v", res, err)
	}
}
