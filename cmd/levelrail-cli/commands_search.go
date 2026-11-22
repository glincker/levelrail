package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

func commandsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s commands search <words...>    find the commands that match, best three first
  %[1]s help <words...>               the same search

Matches against the offline command tree and each command's help text,
for example: %[1]s commands search roll back
`, prog)
}

func runCommands(prog string, args []string, stdout, stderr io.Writer, _ func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		_, _ = fmt.Fprint(stdout, commandsUsage(prog))
		return exitOK
	}
	if args[0] != "search" || len(args) < 2 {
		_, _ = fmt.Fprint(stderr, commandsUsage(prog))
		return exitUsage
	}
	return runCommandSearch(prog, args[1:], stdout)
}

var searchStopWords = map[string]bool{"a": true, "an": true, "the": true, "to": true, "my": true, "how": true, "do": true, "i": true, "of": true, "app": true, "apps": true}

func searchStem(w string) string {
	for _, suf := range []string{"ing", "es", "ed", "s"} {
		if len(w) > len(suf)+2 && strings.HasSuffix(w, suf) {
			return strings.TrimSuffix(w, suf)
		}
	}
	return w
}

// searchTerms lowercases and stems the words and adds each adjacent pair joined, so "roll back" finds "rollback".
func searchTerms(words []string) []string {
	var base []string
	for _, w := range strings.Fields(strings.ToLower(strings.Join(words, " "))) {
		w = strings.Trim(w, `.,?!"'`)
		if w != "" && !searchStopWords[w] {
			base = append(base, w)
		}
	}
	terms := append([]string{}, base...)
	for i := 0; i+1 < len(base); i++ {
		terms = append(terms, base[i]+base[i+1])
	}
	for i, t := range terms {
		terms[i] = searchStem(t)
	}
	return terms
}

type commandHit struct {
	path  string
	usage string
	score int
}

func scoreCommand(path, help string, terms []string) int {
	pathText := strings.ToLower(strings.NewReplacer("-", "", " ", " ").Replace(path))
	pathWords := strings.Fields(strings.ReplaceAll(strings.ToLower(path), "-", " "))
	helpText := strings.ToLower(help)
	score := 0
	for _, t := range terms {
		switch {
		case containsWord(pathWords, t):
			score += 6
		case strings.Contains(pathText, t):
			score += 4
		}
		if n := strings.Count(helpText, t); n > 0 {
			score += 1 + min(n, 3)
		}
	}
	// Deeper paths are narrower, so prefer the shorter one on a tie.
	return score*10 - len(pathWords)
}

func containsWord(words []string, stem string) bool {
	for _, w := range words {
		if searchStem(w) == stem {
			return true
		}
	}
	return false
}

func firstUsageLine(prog, path, help string) string {
	prefix := prog + " " + path
	for _, l := range strings.Split(help, "\n") {
		if t := strings.TrimSpace(l); strings.HasPrefix(t, prefix) {
			return t
		}
	}
	for _, l := range strings.Split(help, "\n") {
		if t := strings.TrimSpace(l); t != "" && !strings.HasPrefix(t, "Usage") {
			return t
		}
	}
	return prefix
}

func searchCommandNodes(prog string, nodes []docsNode, words []string) []commandHit {
	terms := searchTerms(words)
	if len(terms) == 0 {
		return nil
	}
	var hits []commandHit
	for _, n := range nodes {
		path := strings.Join(n.Path, " ")
		if s := scoreCommand(path, n.Help, terms); s > 0 {
			hits = append(hits, commandHit{path: path, usage: firstUsageLine(prog, path, n.Help), score: s})
		}
		// Children are leaf verbs that have no node of their own.
		for _, c := range n.Children {
			cp := path + " " + c
			if s := scoreCommand(cp, "", terms); s > 0 {
				hits = append(hits, commandHit{path: cp, usage: prog + " " + cp, score: s})
			}
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].path < hits[j].path
	})
	seen := map[string]bool{}
	var out []commandHit
	for _, h := range hits {
		if seen[h.path] {
			continue
		}
		seen[h.path] = true
		out = append(out, h)
		if len(out) == 3 {
			break
		}
	}
	return out
}

func runCommandSearch(prog string, words []string, stdout io.Writer) int {
	hits := searchCommandNodes(prog, collectDocs(), words)
	if len(hits) == 0 {
		_, _ = fmt.Fprintf(stdout, "no command matches %q, try \"%s help all\"\n", strings.Join(words, " "), prog)
		return exitOK
	}
	for _, h := range hits {
		_, _ = fmt.Fprintf(stdout, "%s\n", h.usage)
	}
	return exitOK
}
