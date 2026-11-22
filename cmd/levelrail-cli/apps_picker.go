package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

// pickerInput is a variable so tests can feed answers without a terminal.
var pickerInput io.Reader = os.Stdin

// pickerAppNames lists the app names the current credentials can see; tests replace it.
var pickerAppNames = func(prog string, fs *flag.FlagSet) []string {
	lookup := envAliasLookup(rawLookupEnv)
	flagVal := func(name string) string {
		if f := fs.Lookup(name); f != nil {
			return f.Value.String()
		}
		return ""
	}
	profile := resolveProfile(flagVal("profile"), lookup)
	if !savedTokenMayBeSent(prog, flagVal("profile"), flagVal("api-url"), lookup) {
		return nil
	}
	token := resolveToken(flagVal("token"), lookup, prog, profile)
	if token == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), durationEnv(lookup, envCompleteTimeout, defaultCompleteTimeout))
	defer cancel()
	apps, err := NewClient(resolveAPIURL(flagVal("api-url"), lookup, prog, profile), token).ListApps(ctx)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(apps))
	for _, a := range apps {
		names = append(names, a.Name)
	}
	sort.Strings(names)
	return names
}

// filterNames keeps the names containing every space-separated word of q, case-insensitively.
func filterNames(names []string, q string) []string {
	words := strings.Fields(strings.ToLower(q))
	var out []string
	for _, n := range names {
		low := strings.ToLower(n)
		match := true
		for _, w := range words {
			if !strings.Contains(low, w) {
				match = false
				break
			}
		}
		if match {
			out = append(out, n)
		}
	}
	return out
}

// pickName shows a filterable numbered list on stderr and reads answers from in.
// A number picks that row, text narrows the list, a single match is taken on Enter.
func pickName(names []string, in io.Reader, stderr io.Writer, label string) (string, bool) {
	rd := bufio.NewReader(in)
	shown := names
	for {
		for i, n := range shown {
			_, _ = fmt.Fprintf(stderr, "  %2d) %s\n", i+1, n)
		}
		_, _ = fmt.Fprintf(stderr, "Pick %s (number or filter text, empty to cancel): ", label)
		line, err := rd.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			if err == nil {
				_, _ = fmt.Fprintln(stderr)
			}
			return "", false
		}
		if n, convErr := strconv.Atoi(line); convErr == nil {
			if n >= 1 && n <= len(shown) {
				return shown[n-1], true
			}
			_, _ = fmt.Fprintf(stderr, "No row %d.\n", n)
			continue
		}
		matches := filterNames(names, line)
		switch len(matches) {
		case 0:
			_, _ = fmt.Fprintf(stderr, "Nothing matches %q.\n", line)
			shown = names
		case 1:
			return matches[0], true
		default:
			shown = matches
		}
		if err != nil {
			return "", false
		}
	}
}

// pickAppForArg is called when an app argument is missing: on a terminal it offers a picker,
// in CI mode it prints the valid names so the next run can pass one.
func pickAppForArg(fs *flag.FlagSet, stderr io.Writer, prog string) (string, bool) {
	names := pickerAppNames(prog, fs)
	if len(names) == 0 {
		return "", false
	}
	if ciMode {
		_, _ = fmt.Fprintf(stderr, "%s: no app given, valid apps: %s\n", prog, strings.Join(names, ", "))
		return "", false
	}
	return pickName(names, pickerInput, stderr, "an app")
}
