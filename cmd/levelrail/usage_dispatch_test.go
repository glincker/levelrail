package main

import (
	"os"
	"regexp"
	"testing"
)

// A subcommand main dispatches but bootArgsExit rejects dies with exit 2.
func TestEveryDispatchedSubcommandIsAccepted(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	found := regexp.MustCompile(`os\.Args\[1\] == "([a-z-]+)"`).FindAllStringSubmatch(string(src), -1)
	if len(found) == 0 {
		t.Fatal("no dispatched subcommands found in main.go")
	}
	for _, m := range found {
		if !serverSubcommands[m[1]] {
			t.Errorf("main.go dispatches %q but serverSubcommands rejects it", m[1])
		}
	}
}
