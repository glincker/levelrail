package main

import (
	"flag"
	"strings"
)

// reorderArgsFlagsFirst rewrites args so every flag (and its value, if
// it takes one) is moved before any positional argument, then hands
// back a slice fs.Parse can consume normally.
//
// This works around a real stdlib flag.FlagSet limitation: Parse stops
// consuming flags at the first non-flag token, so a natural, common
// invocation shape like "apps get web --json" (name first, flags after,
// the same order `docker inspect <id> --format` or `git show <sha>
// --stat` accept) would otherwise leave --json sitting unparsed in
// fs.Args() instead of setting the flag, silently producing the wrong
// behavior rather than an error. fs must already have every flag it
// will accept defined (via StringVar/BoolVar/etc.) before this is
// called, since that's how this function tells a boolean flag (no
// value token follows it) from a value flag (the next token belongs to
// it, not to the positional arguments).
func reorderArgsFlagsFirst(fs *flag.FlagSet, args []string) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(a) == 0 || a[0] != '-' || a == "-" {
			positional = append(positional, a)
			continue
		}

		flags = append(flags, a)
		name := strings.TrimLeft(a, "-")
		if strings.Contains(name, "=") {
			// "--flag=value" already carries its value in this one
			// token, nothing more to consume.
			continue
		}
		fl := fs.Lookup(name)
		if fl == nil {
			// Unknown flag: leave it to fs.Parse to report the real
			// error rather than guessing whether it takes a value.
			continue
		}
		if bv, ok := fl.Value.(interface{ IsBoolFlag() bool }); ok && bv.IsBoolFlag() {
			continue
		}
		if i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, positional...)
}
