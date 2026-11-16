package main

// extractInsecureHTTPFlag strips --insecure-http anywhere before a "--" terminator so
// every command that builds an API client honours it, not only auth login.
func extractInsecureHTTPFlag(args []string) (remaining []string, set bool) {
	remaining = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			remaining = append(remaining, args[i:]...)
			break
		}
		if args[i] == "--insecure-http" {
			set = true
			continue
		}
		remaining = append(remaining, args[i])
	}
	return remaining, set
}
