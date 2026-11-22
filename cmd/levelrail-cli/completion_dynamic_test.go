package main

import (
	"bytes"
	"reflect"
	"testing"
)

func TestCompletionKindFor(t *testing.T) {
	cases := []struct {
		words []string
		kind  string
		cur   string
	}{
		{[]string{"deploy", "my"}, kindApps, "my"},
		{[]string{"logs", ""}, kindApps, ""},
		{[]string{"apps", "get", "w"}, kindApps, "w"},
		{[]string{"apps", "get", "web", ""}, "", ""},
		{[]string{"apps", "li"}, "", "li"},
		{[]string{"apps", "list", "--profile", "st"}, kindProfiles, "st"},
		{[]string{"deploy", "--env", "pr"}, kindEnvs, "pr"},
		{[]string{"deploy", "--project", "p"}, kindProjects, "p"},
		{[]string{"deploy", "--profile", "x", "we"}, kindApps, "we"},
		{[]string{"deploy", "--js"}, "", "--js"},
	}
	for _, c := range cases {
		kind, cur := completionKindFor(c.words)
		if kind != c.kind || cur != c.cur {
			t.Errorf("%v = (%q, %q), want (%q, %q)", c.words, kind, cur, c.kind, c.cur)
		}
	}
}

func TestHiddenComplete_FiltersByPrefix(t *testing.T) {
	got := completeNames(kindApps, "we", func(string) []string { return []string{"worker", "web", "web-2", "api"} })
	if want := []string{"web", "web-2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	// Offline and unauthenticated, a Tab press prints nothing and exits cleanly.
	var out bytes.Buffer
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if code := runHiddenComplete("levelrail-cli-test-complete", []string{"deploy", ""}, &out, envMap()); code != exitOK || out.Len() != 0 {
		t.Errorf("code=%d out=%q, want a silent success", code, out.String())
	}
}
