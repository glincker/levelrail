package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// Completion runs on every Tab press, so server answers are reused for a short while.
	envCompleteCacheTTL     = "APP_COMPLETE_CACHE_TTL"
	defaultCompleteCacheTTL = 5 * time.Second
	envCompleteTimeout      = "APP_COMPLETE_TIMEOUT"
	defaultCompleteTimeout  = 2 * time.Second

	kindApps     = "apps"
	kindProjects = "projects"
	kindEnvs     = "environments"
	kindProfiles = "profiles"
)

// Flags whose value is a server name worth completing.
var completeFlagKinds = map[string]string{
	"--profile": kindProfiles, "--project": kindProjects,
	"--env": kindEnvs, "--environment": kindEnvs, "--app": kindApps,
}

// Verb paths whose first positional argument is an app name.
var completeAppVerbs = map[string]bool{
	"deploy": true, "logs": true,
	"apps get": true, "apps deploy": true, "apps wait": true, "apps restart": true, "apps stop": true,
	"apps start": true, "apps delete": true, "apps status": true, "apps logs": true, "apps rollback": true,
	"apps scale": true, "apps sleep": true, "apps diagnose": true, "apps metrics": true, "apps exec": true,
	"apps settings": true, "apps timeline": true, "apps revert": true, "apps summary": true,
}

// Flags that take a separate value, so that value is not mistaken for a positional.
var completeValueFlags = map[string]bool{
	"--profile": true, "--project": true, "--env": true, "--environment": true, "--app": true,
	"--api-url": true, "--token": true, "--output": true, "--query": true,
}

// completionKindFor decides which server names fit the cursor position, or "" to let the static completion answer.
// words are everything typed after the program name; the last one is the partial word being completed.
func completionKindFor(words []string) (kind, cur string) {
	if len(words) == 0 {
		return "", ""
	}
	cur = words[len(words)-1]
	before := words[:len(words)-1]
	if len(before) > 0 {
		if k, ok := completeFlagKinds[before[len(before)-1]]; ok {
			return k, cur
		}
	}
	if strings.HasPrefix(cur, "-") {
		return "", cur
	}
	path := ""
	positionals := 0
	skipNext := false
	for _, w := range before {
		if skipNext {
			skipNext = false
			continue
		}
		if strings.HasPrefix(w, "-") {
			skipNext = completeValueFlags[w]
			continue
		}
		if positionals == 0 && isTreeChild(path, w) {
			path = strings.TrimSpace(path + " " + w)
			continue
		}
		positionals++
	}
	if completeAppVerbs[path] && positionals == 0 {
		return kindApps, cur
	}
	return "", cur
}

func isTreeChild(path, name string) bool {
	for _, e := range walkCommandTree() {
		if e.path != path {
			continue
		}
		for _, c := range e.children {
			if c == name {
				return true
			}
		}
	}
	return false
}

// completeNames returns the sorted names of kind that start with cur.
func completeNames(kind, cur string, fetch func(kind string) []string) []string {
	var out []string
	for _, n := range fetch(kind) {
		if strings.HasPrefix(n, cur) {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// runHiddenComplete backs the shell scripts: it prints one candidate per line, nothing when the static completion should answer.
// It never prompts and never fails loudly, because a broken Tab press is worse than no suggestion.
func runHiddenComplete(prog string, args []string, stdout io.Writer, lookupEnv func(string) (string, bool)) int {
	kind, cur := completionKindFor(args)
	if kind == "" {
		return exitOK
	}
	fetch := func(k string) []string { return cachedNames(prog, k, lookupEnv) }
	for _, n := range completeNames(kind, cur, fetch) {
		_, _ = io.WriteString(stdout, n+"\n")
	}
	return exitOK
}

// cachedNames answers from a short-lived file in the user cache dir, then from the server.
func cachedNames(prog, kind string, lookupEnv func(string) (string, bool)) []string {
	profile := resolveProfile("", lookupEnv)
	if kind == kindProfiles {
		return profileNames(prog)
	}
	ttl := durationEnv(lookupEnv, envCompleteCacheTTL, defaultCompleteCacheTTL)
	path := completeCachePath(prog, profile, kind)
	if st, err := os.Stat(path); err == nil && time.Since(st.ModTime()) < ttl {
		if raw, rerr := os.ReadFile(path); rerr == nil { //nolint:gosec // path is built from the user cache dir and sanitized parts
			return splitLines(string(raw))
		}
	}
	names := fetchNames(prog, profile, kind, lookupEnv)
	if names != nil && path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err == nil {
			_ = os.WriteFile(path, []byte(strings.Join(names, "\n")), 0o600)
		}
	}
	return names
}

func profileNames(prog string) []string {
	ps, err := listProfiles(prog)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.Name)
	}
	return out
}

func fetchNames(prog, profile, kind string, lookupEnv func(string) (string, bool)) []string {
	if !savedTokenMayBeSent(prog, "", "", lookupEnv) {
		return nil
	}
	token := resolveToken("", lookupEnv, prog, profile)
	if token == "" {
		return nil
	}
	client := NewClient(resolveAPIURL("", lookupEnv, prog, profile), token)
	ctx, cancel := context.WithTimeout(context.Background(), durationEnv(lookupEnv, envCompleteTimeout, defaultCompleteTimeout))
	defer cancel()
	var names []string
	switch kind {
	case kindApps:
		apps, err := client.ListApps(ctx)
		if err != nil {
			return nil
		}
		for _, a := range apps {
			names = append(names, a.Name)
		}
	case kindProjects:
		ps, err := client.ListProjects(ctx)
		if err != nil {
			return nil
		}
		for _, p := range ps {
			names = append(names, p.Name)
		}
	case kindEnvs:
		es, err := client.ListAllEnvironments(ctx)
		if err != nil {
			return nil
		}
		seen := map[string]bool{}
		for _, e := range es {
			if !seen[e.Name] {
				seen[e.Name] = true
				names = append(names, e.Name)
			}
		}
	}
	return names
}

func completeCachePath(prog, profile, kind string) string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, sanitizeIdent(prog), "complete-"+sanitizeIdent(profile)+"-"+kind+".txt")
}

func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}
