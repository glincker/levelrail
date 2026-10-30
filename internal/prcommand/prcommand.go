// Package prcommand parses /levelrail commands out of a pull request comment.
package prcommand

import (
	"errors"
	"strings"
)

// Command is one accepted command.
type Command string

// The accepted commands. Nothing else parses.
const (
	Redeploy        Command = "redeploy"
	Rollback        Command = "rollback"
	Status          Command = "status"
	PreviewRedeploy Command = "preview redeploy"
	Help            Command = "help"
)

// Supported returns every accepted command in the order help lists them.
func Supported() []Command {
	return []Command{Status, Redeploy, Rollback, PreviewRedeploy, Help}
}

// Prefix starts every command line.
const Prefix = "/levelrail"

// ErrMultiple means the comment holds more than one command line. Acting on
// one of several would be a guess, so none runs.
var ErrMultiple = errors.New("prcommand: more than one command in the comment")

// Parse returns the single command in body, or "" when there is none.
// A command is a line that starts with the prefix at column zero, outside
// fenced code, with nothing after the verb. Quoted replies, indented code and
// prose mentions never match. Unknown verbs and trailing text are ignored.
func Parse(body string) (Command, error) {
	var found Command
	count := 0
	fence := ""
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if marker := fenceMarker(line); marker != "" {
			switch {
			case fence == "":
				fence = marker
			case strings.HasPrefix(marker, fence[:1]) && len(marker) >= len(fence):
				fence = ""
			}
			continue
		}
		if fence != "" {
			continue
		}
		if c, ok := parseLine(line); ok {
			count++
			found = c
		}
	}
	if count > 1 {
		return "", ErrMultiple
	}
	return found, nil
}

// fenceMarker returns the run of backticks or tildes that opens or closes a
// fenced block (up to three spaces of indent allowed), else "".
func fenceMarker(line string) string {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 || trimmed == "" {
		return ""
	}
	ch := trimmed[0]
	if ch != '`' && ch != '~' {
		return ""
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == ch {
		n++
	}
	if n < 3 {
		return ""
	}
	return trimmed[:n]
}

func parseLine(line string) (Command, bool) {
	if len(line) < len(Prefix) || !strings.EqualFold(line[:len(Prefix)], Prefix) {
		return "", false
	}
	rest := line[len(Prefix):]
	if rest == "" || (rest[0] != ' ' && rest[0] != '\t') {
		return "", false
	}
	words := strings.Fields(strings.ToLower(rest))
	switch strings.Join(words, " ") {
	case string(Redeploy):
		return Redeploy, true
	case string(Rollback):
		return Rollback, true
	case string(Status):
		return Status, true
	case string(PreviewRedeploy):
		return PreviewRedeploy, true
	case string(Help):
		return Help, true
	}
	return "", false
}
