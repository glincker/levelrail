package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/envrules"
)

// envLineIssue is one refused line or key. Line is 0 when the issue is about a key, not a line.
type envLineIssue struct {
	Line   int    `json:"line,omitempty"`
	Key    string `json:"key,omitempty"`
	Reason string `json:"reason"`
}

// envImportReport says what a dotenv import took and what it refused; names and line numbers, never values.
type envImportReport struct {
	Accepted []string       `json:"accepted"`
	Rejected []envLineIssue `json:"rejected"`
	Warnings []envLineIssue `json:"warnings"`
}

// scanEnvFile parses like parseEnvFileBytes and also records malformed lines, duplicates and keys the env rules refuse.
// Refused keys are left out of the returned entries so they are never written.
func scanEnvFile(data []byte) ([]envFileEntry, envImportReport) {
	rep := envImportReport{Accepted: []string{}, Rejected: []envLineIssue{}, Warnings: []envLineIssue{}}
	s := strings.ReplaceAll(strings.TrimPrefix(string(data), "\xef\xbb\xbf"), "\r\n", "\n")
	var entries []envFileEntry
	lines := map[string][]int{}
	lineOf := map[string]int{}
	lineNo := 0
	for s != "" {
		lineNo++
		var line string
		before := s
		line, s = cutEnvLine(s)
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "export "); ok {
			line = strings.TrimSpace(rest)
		}
		eq := strings.Index(line, "=")
		if eq == -1 {
			rep.Rejected = append(rep.Rejected, envLineIssue{Line: lineNo, Reason: "not a KEY=VALUE line"})
			continue
		}
		key := strings.TrimSpace(line[:eq])
		if !validEnvKey(key) {
			rep.Rejected = append(rep.Rejected, envLineIssue{Line: lineNo, Key: shortKey(key), Reason: "not a valid variable name"})
			continue
		}
		var value string
		value, s = parseEnvValue(strings.TrimSpace(line[eq+1:]), s)
		entries = append(entries, envFileEntry{Key: key, Value: value})
		lines[key] = append(lines[key], lineNo)
		lineOf[key] = lineNo
		// A multi-line quoted value consumed extra lines.
		lineNo += textLines(before) - textLines(s) - 1
	}
	for k, ls := range lines {
		if len(ls) > 1 {
			rep.Warnings = append(rep.Warnings, envLineIssue{Line: ls[0], Key: k, Reason: fmt.Sprintf("defined %d times (lines %s), the last value wins", len(ls), joinInts(ls))})
		}
	}
	set := map[string]string{}
	for _, e := range entries {
		set[e.Key] = e.Value
	}
	res := envrules.Validate(set, envrules.Existing{}, envrules.DefaultPolicy(), envrules.Options{ExistingUnknown: true})
	refused := map[string]bool{}
	for _, e := range res.Errors {
		refused[e.Key] = true
		rep.Rejected = append(rep.Rejected, envLineIssue{Line: lineOf[e.Key], Key: e.Key, Reason: e.Message})
	}
	for _, w := range res.Warnings {
		rep.Warnings = append(rep.Warnings, envLineIssue{Line: lineOf[w.Key], Key: w.Key, Reason: w.Message})
	}
	kept := entries[:0:0]
	seen := map[string]bool{}
	for _, e := range entries {
		if refused[e.Key] {
			continue
		}
		kept = append(kept, e)
		if !seen[e.Key] {
			seen[e.Key] = true
			rep.Accepted = append(rep.Accepted, e.Key)
		}
	}
	sort.Strings(rep.Accepted)
	sort.SliceStable(rep.Rejected, func(i, j int) bool { return rep.Rejected[i].Line < rep.Rejected[j].Line })
	sort.SliceStable(rep.Warnings, func(i, j int) bool { return rep.Warnings[i].Line < rep.Warnings[j].Line })
	return kept, rep
}

func textLines(x string) int {
	n := strings.Count(x, "\n")
	if x != "" && !strings.HasSuffix(x, "\n") {
		n++
	}
	return n
}

// shortKey keeps a malformed name readable without echoing a long line.
func shortKey(k string) string {
	if len(k) > 40 || strings.ContainsAny(k, " \t") {
		return ""
	}
	return k
}

func joinInts(ls []int) string {
	parts := make([]string, len(ls))
	for i, n := range ls {
		parts[i] = fmt.Sprint(n)
	}
	return strings.Join(parts, ", ")
}

func printEnvImportReport(w io.Writer, rep envImportReport) {
	_, _ = fmt.Fprintf(w, "%d key(s) accepted, %d rejected, %d warning(s)\n", len(rep.Accepted), len(rep.Rejected), len(rep.Warnings))
	for _, r := range rep.Rejected {
		_, _ = fmt.Fprintf(w, "x %s\n", describeEnvIssue(r))
	}
	for _, r := range rep.Warnings {
		_, _ = fmt.Fprintf(w, "? %s\n", describeEnvIssue(r))
	}
}

func describeEnvIssue(r envLineIssue) string {
	where := ""
	if r.Line > 0 {
		where = fmt.Sprintf("line %d", r.Line)
	}
	if r.Key != "" {
		if where != "" {
			where += " "
		}
		where += r.Key
	}
	if where == "" {
		return r.Reason
	}
	return where + ": " + r.Reason
}
