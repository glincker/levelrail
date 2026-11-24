package lognorm

import (
	"regexp"
	"strconv"
	"strings"
)

// Caps for one joined record. Package constants until the wiring slice
// makes them runtime settings.
const (
	MaxStackLines   = 64
	MaxRecordBytes  = 8192
	truncatedMarker = "[truncated "
)

type stackKind int

const (
	kindNone stackKind = iota
	kindGo
	kindJava
	kindPython
)

var (
	goStartRe   = regexp.MustCompile(`^(panic: |fatal error: |goroutine \d+ \[|runtime: )`)
	goFuncRe    = regexp.MustCompile(`^[\w./*()\[\]-]+\(.*\)$`)
	goExtraRe   = regexp.MustCompile(`^(created by |exit status |\[signal )`)
	javaStartRe = regexp.MustCompile(`^(Exception in thread |[\w$.]+(Exception|Error)(:|$))`)
	javaContRe  = regexp.MustCompile(`^(\s+at |\s*\.\.\. \d+ (more|common)|Caused by: |Suppressed: |\s+Suppressed)`)
	atLineRe    = regexp.MustCompile(`^\s+at \S`)
	pyStartRe   = regexp.MustCompile(`^Traceback \(most recent call last\):`)
	pyFinalRe   = regexp.MustCompile(`^[\w.]+(Error|Exception|Interrupt|Exit)\b|^[\w.]+: `)
	pyChainRe   = regexp.MustCompile(`^(During handling of the above exception|The above exception was the direct cause)`)
)

// Join folds Go, Java, Python and Node stack traces into single multi-line
// records and passes every other line through untouched.
func Join(lines []string) []string {
	var out []string
	var cur []string
	kind := kindNone
	pyNeedFinal := false

	flush := func() {
		if len(cur) > 0 {
			out = append(out, finish(cur))
		}
		cur, kind, pyNeedFinal = nil, kindNone, false
	}

	for i, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		if kind != kindNone {
			if continues(kind, line, &pyNeedFinal) {
				cur = append(cur, line)
				if kind == kindPython && !pyNeedFinal && cur[len(cur)-1] == line && pyFinalRe.MatchString(line) && !pyChainRe.MatchString(line) && !strings.HasPrefix(line, " ") {
					flush()
				}
				continue
			}
			flush()
		}
		switch {
		case goStartRe.MatchString(line):
			kind, cur = kindGo, []string{line}
		case pyStartRe.MatchString(line):
			kind, cur, pyNeedFinal = kindPython, []string{line}, true
		case javaStartRe.MatchString(line) || (i+1 < len(lines) && atLineRe.MatchString(lines[i+1]) && !atLineRe.MatchString(line)):
			kind, cur = kindJava, []string{line}
		default:
			out = append(out, line)
		}
	}
	flush()
	return out
}

func continues(kind stackKind, line string, pyNeedFinal *bool) bool {
	switch kind {
	case kindGo:
		return strings.HasPrefix(line, "\t") || strings.HasPrefix(line, " ") || line == "" ||
			goFuncRe.MatchString(line) || goExtraRe.MatchString(line) || goStartRe.MatchString(line)
	case kindJava:
		return javaContRe.MatchString(line)
	case kindPython:
		if pyChainRe.MatchString(line) || strings.HasPrefix(line, " ") || line == "" {
			return true
		}
		if *pyNeedFinal && pyFinalRe.MatchString(line) {
			*pyNeedFinal = false
			return true
		}
		if pyStartRe.MatchString(line) {
			*pyNeedFinal = true
			return true
		}
	}
	return false
}

// finish joins a record, dropping the middle of oversized traces but keeping
// the head and the last line (usually the root cause or exit status).
func finish(lines []string) string {
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > MaxStackLines {
		drop := len(lines) - MaxStackLines
		kept := append([]string{}, lines[:MaxStackLines-1]...)
		kept = append(kept, truncatedMarker+strconv.Itoa(drop)+" lines]", lines[len(lines)-1])
		lines = kept
	}
	s := strings.Join(lines, "\n")
	if len(s) > MaxRecordBytes {
		s = s[:MaxRecordBytes] + "\n" + truncatedMarker + "bytes]"
	}
	return s
}
