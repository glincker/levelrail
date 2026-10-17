package main

// Strips --debug from anywhere in args, but never past a literal '--' passthrough separator.
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
