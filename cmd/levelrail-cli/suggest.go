package main

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/GLINCKER/levelrail/internal/clitree"
)

// cliProg is the binary name for empty-result hints that have no prog argument.
var cliProg = "levelrail-cli"

// closestMatches returns up to three candidates near got, best first. The
// allowed distance scales with length so short verbs do not match noise.
func closestMatches(candidates []string, got string) []string {
	limit := 1
	switch {
	case len(got) >= 8:
		limit = 3
	case len(got) >= 4:
		limit = 2
	}
	type scored struct {
		name string
		dist int
	}
	var hits []scored
	seen := map[string]bool{}
	for _, c := range candidates {
		if seen[c] {
			continue
		}
		seen[c] = true
		d := editDistance(strings.ToLower(got), strings.ToLower(c))
		if d == 2 && sameLetters(got, c) {
			d = 1 // a swapped pair of letters is one typo
		}
		if d <= limit || (len(got) >= 3 && strings.HasPrefix(c, got)) {
			hits = append(hits, scored{c, d})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].dist != hits[j].dist {
			return hits[i].dist < hits[j].dist
		}
		return hits[i].name < hits[j].name
	})
	var out []string
	for _, h := range hits {
		if len(out) == 3 {
			break
		}
		out = append(out, h.name)
	}
	return out
}

// verbCandidates lists the verbs the command tree accepts under path.
func verbCandidates(path []string) []string {
	subs := clitree.Tree
	for _, p := range path {
		n := subs[p]
		if n == nil {
			return nil
		}
		subs = n.Subs
	}
	names := make([]string, 0, len(subs))
	for name := range subs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// unknownVerbHint is the shared next-step text for a mistyped verb: a
// did-you-mean line when something is close, then where to look next.
func unknownVerbHint(prog string, path []string, got string) string {
	var b strings.Builder
	if m := closestMatches(verbCandidates(path), got); len(m) > 0 {
		full := strings.TrimSpace(strings.Join(path, " "))
		if full != "" {
			full += " "
		}
		opts := make([]string, len(m))
		for i, s := range m {
			opts[i] = prog + " " + full + s
		}
		fmt.Fprintf(&b, "did you mean: %s?\n", strings.Join(opts, ", "))
	}
	helpPath := strings.TrimSpace(strings.Join(path, " "))
	if helpPath != "" {
		helpPath += " "
	}
	fmt.Fprintf(&b, "next step: run %s %s--help to list the available commands\n", prog, helpPath)
	return b.String()
}

// unknownCommandHint extends the top-level hint: when the typed noun is
// close to a real one and the next word is a verb of it, suggest the pair.
func unknownCommandHint(prog string, args []string) string {
	if len(args) > 1 && !strings.HasPrefix(args[1], "-") {
		for _, noun := range closestMatches(verbCandidates(nil), args[0]) {
			for _, v := range verbCandidates([]string{noun}) {
				if v == args[1] {
					return fmt.Sprintf("did you mean: %s %s %s?\n", prog, noun, v) +
						fmt.Sprintf("next step: run %s %s --help to list the available commands\n", prog, noun)
				}
			}
		}
	}
	return unknownVerbHint(prog, nil, args[0])
}

var (
	badFlagRe  = regexp.MustCompile(`flag provided but not defined: -{1,2}([A-Za-z0-9][\w.-]*)`)
	usageFlagR = regexp.MustCompile(`(?m)^\s+-{1,2}([A-Za-z0-9][\w.-]*)`)
)

// hintWriter passes stderr through and remembers the tail, so a stdlib
// "flag provided but not defined" can be followed by a did-you-mean line
// built from the flags the command's own usage just listed.
type hintWriter struct {
	mu   sync.Mutex
	dst  io.Writer
	seen []byte
}

const hintWriterCap = 64 << 10

func (h *hintWriter) Write(p []byte) (int, error) {
	h.mu.Lock()
	if len(h.seen) < hintWriterCap {
		h.seen = append(h.seen, p...)
	}
	h.mu.Unlock()
	return h.dst.Write(p)
}

// flagHint is empty unless the recorded stderr holds a rejected flag.
func (h *hintWriter) flagHint(prog string, args []string) string {
	h.mu.Lock()
	text := string(h.seen)
	h.mu.Unlock()
	m := badFlagRe.FindStringSubmatchIndex(text)
	if m == nil {
		return ""
	}
	bad := text[m[2]:m[3]]
	var known []string
	for _, f := range usageFlagR.FindAllStringSubmatch(text[m[1]:], -1) {
		known = append(known, f[1])
	}
	var b strings.Builder
	if near := closestMatches(known, bad); len(near) > 0 {
		for i := range near {
			near[i] = "--" + near[i]
		}
		fmt.Fprintf(&b, "did you mean: %s?\n", strings.Join(near, ", "))
	}
	var verbs []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			break
		}
		verbs = append(verbs, a)
	}
	fmt.Fprintf(&b, "next step: run %s %s --help to see every flag\n", prog, strings.Join(verbs, " "))
	return b.String()
}

// emptyHint turns a bare "no X" into a next step; when a project scope is
// active it also says how to see every project.
func emptyHint(noun, createCmd string) string {
	msg := fmt.Sprintf("no %s yet: run %s %s", noun, cliProg, createCmd)
	if p := scopeFlags.Project; p != "" && p != allProjectsMarker {
		msg = fmt.Sprintf("no %s in project %q: add -A to list every project, or run %s %s", noun, p, cliProg, createCmd)
	}
	return msg
}

// undoHint is the one-line text-mode tail naming the exact command that
// reverses a state change; args are shell-quoted when they need it.
func undoHint(prog string, args ...string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = shellWord(a)
	}
	return fmt.Sprintf("undo: %s %s\n", prog, strings.Join(parts, " "))
}

// shellWord single-quotes a word that holds anything outside a safe set.
func shellWord(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-./:@=,") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func sameLetters(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	x, y := []byte(strings.ToLower(a)), []byte(strings.ToLower(b))
	sort.Slice(x, func(i, j int) bool { return x[i] < x[j] })
	sort.Slice(y, func(i, j int) bool { return y[i] < y[j] })
	return string(x) == string(y)
}

// errorNextStep is the one-line text-mode tail for the most common API
// failures; empty when no generic advice fits.
func errorNextStep(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return cliProg + " login, or pass --token"
	case http.StatusForbidden:
		return "this token lacks the ability named above; use a token that has it, " + cliProg + " whoami shows what the current one can do"
	case http.StatusNotFound:
		return "check the name with the list verb, for example " + cliProg + " apps list"
	case http.StatusUnprocessableEntity, http.StatusBadRequest:
		return "fix the flagged input and retry, see the verb's --help"
	}
	return ""
}

// errorNextStepFor narrows the 404 advice to the resource named in the error text,
// so a missing template or node does not point at the apps list.
func errorNextStepFor(status int, msg string) string {
	if status == http.StatusNotFound {
		switch {
		case strings.Contains(msg, "template"):
			return "check the name with " + cliProg + " templates list"
		case strings.Contains(msg, "zone"):
			return "check the zone with " + cliProg + " nodes zones"
		case strings.Contains(msg, "node"):
			return "check the node id with " + cliProg + " nodes list"
		}
	}
	return errorNextStep(status)
}

// splitInlineNext separates a trailing ". next: ..." step from an error message
// so it prints on its own line like every other next step.
func splitInlineNext(msg string) (string, string) {
	if i := strings.Index(msg, ". next: "); i >= 0 {
		return msg[:i], msg[i+len(". next: "):]
	}
	return msg, ""
}

// errorHasGuidance reports whether the error text already says what to do,
// so a generic footer would only add noise (login and approval flows).
func errorHasGuidance(msg string) bool {
	for _, m := range []string{"approve at", "run on", "fix:", "hint:", "http://", "https://", "login"} {
		if strings.Contains(msg, m) {
			return true
		}
	}
	return false
}
