package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/cliupdate"
)

// hintSkipVerbs never carry the hint: they are about versions already, or are scripted by tools.
var hintSkipVerbs = map[string]bool{"help": true, "completion": true, "version": true, "update": true, "-h": true, "--help": true}

func wantsJSONOutput(args []string) bool {
	for i, a := range args {
		switch {
		case a == "--json", a == "-json", a == "--output=json", a == "-output=json":
			return true
		case (a == "--output" || a == "-output") && i+1 < len(args) && args[i+1] == "json":
			return true
		}
	}
	return false
}

// startUpdateHint begins a background check and returns a function that, once
// the command has finished, waits a short capped time and prints at most one
// stderr line. It never delays the command itself.
func startUpdateHint(prog string, args []string, stderr io.Writer, lookupEnv func(string) (string, bool)) func() {
	noop := func() {}
	if len(args) == 0 || hintSkipVerbs[args[0]] || wantsJSONOutput(args) {
		return noop
	}
	if forceCI || ciEnv(lookupEnv) {
		return noop
	}
	if v, ok := lookupEnv(envNoUpdateCheck); ok && truthy(v) {
		return noop
	}
	days := updateIntervalDays(lookupEnv)
	dir, err := configDir(prog)
	if days <= 0 || err != nil {
		return noop
	}
	src, err := updateSource(lookupEnv)
	if err != nil {
		return noop
	}
	wait := time.Duration(defaultUpdateHintWaitMS) * time.Millisecond
	if v, ok := lookupEnv(envUpdateHintWaitMS); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			wait = time.Duration(n) * time.Millisecond
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	result := make(chan string, 1)
	hinter := cliupdate.Hinter{Source: src, Current: localVersionInfo().Version, Dir: dir, IntervalDays: days}
	go func() { result <- hinter.Check(ctx) }()
	return func() {
		defer cancel()
		select {
		case line := <-result:
			if line != "" {
				_, _ = fmt.Fprintf(stderr, "%s Run: %s update\n", line, prog)
			}
		case <-ctx.Done():
		}
	}
}
