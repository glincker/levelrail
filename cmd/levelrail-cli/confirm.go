package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// extractNoInputFlag strips the global --no-input, which turns on CI mode so nothing prompts.
func extractNoInputFlag(args []string) (remaining []string, set bool) {
	remaining = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			remaining = append(remaining, args[i:]...)
			break
		}
		if args[i] == "--no-input" {
			set = true
			continue
		}
		remaining = append(remaining, args[i])
	}
	return remaining, set
}

// confirmAction is the one confirm rule: --yes skips it, CI mode or a non-terminal stdin
// fails fast naming the flag, otherwise the user is asked y/N on stderr.
func confirmAction(stdin io.Reader, stderr io.Writer, question string, yes bool, yesFlag string) error {
	if yes {
		return nil
	}
	if f, ok := stdin.(*os.File); ciMode || !ok || !term.IsTerminal(int(f.Fd())) { //nolint:gosec // a terminal file descriptor always fits an int
		return newValidationError("%s needs confirmation but input is disabled: pass %s", question, yesFlag)
	}
	_, _ = fmt.Fprintf(stderr, "%s? [y/N] ", question)
	line, _ := bufio.NewReader(stdin).ReadString('\n')
	if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
		return newValidationError("cancelled")
	}
	return nil
}
