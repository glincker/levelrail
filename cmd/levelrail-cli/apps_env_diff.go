package main

import (
	"context"
	"fmt"
	"io"
	"sort"
)

type envDiffEntry struct {
	Key    string `json:"key"`
	A      string `json:"a,omitempty"`
	B      string `json:"b,omitempty"`
	Secret bool   `json:"secret,omitempty"`
}

type envDiffResult struct {
	A      string         `json:"a"`
	B      string         `json:"b"`
	OnlyA  []envDiffEntry `json:"only_a"`
	OnlyB  []envDiffEntry `json:"only_b"`
	Differ []envDiffEntry `json:"differ"`
	Same   int            `json:"same"`
}

// diffEnv compares two apps' env. Secret values are never read: a secret key
// is only compared by presence, and a key that is secret on one side and
// plain on the other counts as differing.
func diffEnv(aName string, aEnv map[string]string, aSecrets []string, bName string, bEnv map[string]string, bSecrets []string) envDiffResult {
	res := envDiffResult{A: aName, B: bName, OnlyA: []envDiffEntry{}, OnlyB: []envDiffEntry{}, Differ: []envDiffEntry{}}
	aSec, bSec := toSet(aSecrets), toSet(bSecrets)
	keys := map[string]bool{}
	for k := range aEnv {
		keys[k] = true
	}
	for k := range bEnv {
		keys[k] = true
	}
	for k := range aSec {
		keys[k] = true
	}
	for k := range bSec {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, k := range sorted {
		_, inA := aEnv[k]
		_, inB := bEnv[k]
		aHas, bHas := inA || aSec[k], inB || bSec[k]
		secret := aSec[k] || bSec[k]
		switch {
		case aHas && !bHas:
			res.OnlyA = append(res.OnlyA, envDiffEntry{Key: k, A: plainValue(aEnv, k, secret), Secret: secret})
		case bHas && !aHas:
			res.OnlyB = append(res.OnlyB, envDiffEntry{Key: k, B: plainValue(bEnv, k, secret), Secret: secret})
		case aSec[k] != bSec[k]:
			res.Differ = append(res.Differ, envDiffEntry{Key: k, Secret: true})
		case secret || aEnv[k] == bEnv[k]:
			res.Same++
		default:
			res.Differ = append(res.Differ, envDiffEntry{Key: k, A: aEnv[k], B: bEnv[k]})
		}
	}
	return res
}

func plainValue(env map[string]string, key string, secret bool) string {
	if secret {
		return ""
	}
	return env[key]
}

func toSet(keys []string) map[string]bool {
	out := make(map[string]bool, len(keys))
	for _, k := range keys {
		out[k] = true
	}
	return out
}

func printEnvDiff(out io.Writer, r envDiffResult) {
	_, _ = fmt.Fprintf(out, "--- %s\n+++ %s\n", r.A, r.B)
	for _, e := range r.OnlyA {
		_, _ = fmt.Fprintf(out, "- %s%s\n", e.Key, envDiffValue(e.A, e.Secret))
	}
	for _, e := range r.OnlyB {
		_, _ = fmt.Fprintf(out, "+ %s%s\n", e.Key, envDiffValue(e.B, e.Secret))
	}
	for _, e := range r.Differ {
		if e.Secret {
			_, _ = fmt.Fprintf(out, "~ %s (secret on one side only)\n", e.Key)
			continue
		}
		_, _ = fmt.Fprintf(out, "~ %s=%s -> %s\n", e.Key, e.A, e.B)
	}
	_, _ = fmt.Fprintf(out, "%d identical, %d only in %s, %d only in %s, %d differ\n", r.Same, len(r.OnlyA), r.A, len(r.OnlyB), r.B, len(r.Differ))
}

func envDiffValue(v string, secret bool) string {
	if secret {
		return " (secret)"
	}
	return "=" + v
}

func runAppsEnvDiff(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps env diff", "print the diff as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps env diff <app-a> <app-b> [flags]\n\nCompares two apps' env vars, for example staging against production.\nSecret values are never read, secrets are compared by key only.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: apps env diff takes exactly two app names\n", prog)
		return exitUsage
	}
	nameA, nameB := fs.Arg(0), fs.Arg(1)

	ctx := context.Background()
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	load := func(name string) (map[string]string, []string, error) {
		app, err := client.GetApp(ctx, name)
		if err != nil {
			return nil, nil, fmt.Errorf("get app %q: %w", name, err)
		}
		keys, err := appSecretKeys(ctx, client, name, app.SecretEnv)
		return app.Env, keys, err
	}
	envA, secA, err := load(nameA)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	envB, secB, err := load(nameB)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	res := diffEnv(nameA, envA, secA, nameB, envB, secB)
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printEnvDiff(stdout, res) })
}
