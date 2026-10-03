package main

// extractDebugFlag strips a top-level "--debug" switch from args before
// any subcommand's flag.FlagSet sees them, so it works anywhere on the
// line. Stops at a literal "--": everything after that belongs to a
// passthrough command like "apps exec <name> -- <cmd> [args...]", never
// this CLI's own flags.
func extractDebugFlag(args []string) (remaining []string, debug bool) {
	remaining = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			remaining = append(remaining, args[i:]...)
			break
		}
		if args[i] == "--debug" {
			debug = true
			continue
		}
		remaining = append(remaining, args[i])
	}
	return remaining, debug
}
