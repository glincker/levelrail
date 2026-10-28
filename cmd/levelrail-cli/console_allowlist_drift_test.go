package main

import (
	"bytes"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/cliconsole"
)

var (
	helpFlagLine = regexp.MustCompile(`(?m)^\s+--?([A-Za-z0-9][A-Za-z0-9_-]*)`)
	// riskyFlag is deliberately broader than the console's own deny rules, so a
	// path, URL or launch flag on an allowlisted command forces a review here.
	riskyFlag = regexp.MustCompile(`file|path|dir|input|output|^out|include|exclude|manifest|signature|cert|bin|image|launch|csv|ratchet|record|^fix|config|script|exec|cmd|command|socket|local|stdin|dockerfile|context|kube|upload|download|save|write|url|endpoint|webhook|from`)
)

// reviewedSafeFlags are risky-looking names on allowlisted commands that only
// filter or format an API response; each one was checked by hand.
var reviewedSafeFlags = map[string]string{
	"from": "a start time for a logs, metrics or traces window",
}

// TestConsoleAllowlistFlagsReviewed runs -h for every allowlisted console
// command and fails on a path, URL or launch style flag the console still accepts.
func TestConsoleAllowlistFlagsReviewed(t *testing.T) {
	noEnv := func(string) (string, bool) { return "", false }
	withFlags := 0
	for _, e := range cliconsole.Allowlist() {
		var out, errb bytes.Buffer
		args := append(strings.Fields(e.Path), "-h")
		_ = run("cli", args, &out, &errb, noEnv)
		matches := helpFlagLine.FindAllStringSubmatch(out.String()+errb.String(), -1)
		if len(matches) > 0 {
			withFlags++
		}
		for _, m := range matches {
			name := m[1]
			if !riskyFlag.MatchString(name) || cliconsole.FlagDenied(name) {
				continue
			}
			if _, ok := reviewedSafeFlags[name]; ok {
				continue
			}
			t.Errorf("%q accepts --%s, which looks like a host path, URL or launch flag: deny it in the console or review it in reviewedSafeFlags", e.Path, name)
		}
	}
	// Guards the guard: if -h stops printing flags, this test would pass blind.
	if withFlags*2 < len(cliconsole.Allowlist()) {
		t.Errorf("only %d of %d allowlisted commands printed flags for -h", withFlags, len(cliconsole.Allowlist()))
	}
	t.Logf("%d of %d allowlisted commands printed flags", withFlags, len(cliconsole.Allowlist()))
}

// TestConsoleScopeStripMatchesCLI keeps the console's --team/--project strip
// in step with the CLI's own, so Check vets the command the CLI will run.
func TestConsoleScopeStripMatchesCLI(t *testing.T) {
	for _, e := range cliconsole.Allowlist() {
		words := strings.Fields(e.Path)
		cases := [][]string{
			append([]string{"--team", "t1"}, words...),
			append([]string{"--project=p1"}, words...),
			append([]string{"--project", "p1", "--team=t1"}, words...),
			append(append([]string(nil), words...), "web", "--team", "t1"),
			append(append([]string(nil), words...), "--project", "p1", "web"),
			append(append([]string(nil), words...), "--team"),
		}
		for _, argv := range cases {
			want, _ := extractScopeFlags(argv)
			if got := cliconsole.DispatchArgs(argv); !reflect.DeepEqual(got, want) {
				t.Errorf("DispatchArgs(%q) = %q, CLI dispatches %q", argv, got, want)
			}
		}
	}
}
