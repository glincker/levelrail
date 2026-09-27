package main

import (
	"bytes"
	"sort"
	"strings"
	"testing"
)

// jsonExempt lists leaf commands that intentionally have no --json, each
// with the reason. Keep it short: a command that returns data or a status
// an agent would branch on belongs on --json, not in this list.
var jsonExempt = map[string]string{
	"completion bash":               "prints a shell script meant to be sourced",
	"completion zsh":                "prints a shell script meant to be sourced",
	"completion fish":               "prints a shell script meant to be sourced",
	"control-plane-backups help-dr": "prints a static runbook, no data to structure",
}

func leafPaths(prefix []string, nodes map[string]*cmdNode) [][]string {
	var out [][]string
	for name, n := range nodes {
		path := append(append([]string{}, prefix...), name)
		if n == nil || len(n.subs) == 0 {
			out = append(out, path)
			continue
		}
		out = append(out, leafPaths(path, n.subs)...)
	}
	return out
}

func TestEveryLeafCommandSupportsJSON(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	leaves := leafPaths(nil, cliCommandTree)
	sort.Slice(leaves, func(i, j int) bool { return strings.Join(leaves[i], " ") < strings.Join(leaves[j], " ") })
	var missing []string
	for _, leaf := range leaves {
		key := strings.Join(leaf, " ")
		var stdout, stderr bytes.Buffer
		lookup := func(k string) (string, bool) {
			if k == envAPIURL {
				return "http://127.0.0.1:1", true
			}
			return "", false
		}
		run("levelrail-cli", append(append([]string{}, leaf...), "--json"), &stdout, &stderr, lookup)
		has := !strings.Contains(stdout.String()+stderr.String(), "flag provided but not defined: -json")
		var probeOut, probeErr bytes.Buffer
		run("levelrail-cli", append(append([]string{}, leaf...), "--no-such-flag-probe"), &probeOut, &probeErr, lookup)
		if !strings.Contains(probeOut.String()+probeErr.String(), "flag provided but not defined: -no-such-flag-probe") {
			has = false
		}
		reason, exempt := jsonExempt[key]
		switch {
		case has && exempt:
			t.Errorf("%q supports --json but is listed in jsonExempt (%s): remove it", key, reason)
		case !has && !exempt:
			missing = append(missing, key)
		}
	}
	for _, key := range missing {
		t.Errorf("%q has no --json flag: add it or list it in jsonExempt with a reason", key)
	}
	for key := range jsonExempt {
		found := false
		for _, leaf := range leaves {
			if strings.Join(leaf, " ") == key {
				found = true
			}
		}
		if !found {
			t.Errorf("jsonExempt entry %q is not a command", key)
		}
	}
}
