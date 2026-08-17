package main

import (
	"fmt"
	"io"
)

// runMigrate dispatches "migrate <source> [flags]".
func runMigrate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, migrateUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, migrateUsage(prog))
		return exitOK
	case "coolify":
		return runMigrateCoolify(prog, args[1:], stdout, stderr, lookupEnv)
	case "dokploy":
		return runMigrateDokploy(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown migrate source %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, migrateUsage(prog))
		return exitUsage
	}
}

func migrateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s migrate coolify --url URL --token TOKEN [flags]   migrate apps from a Coolify instance
  %[1]s migrate dokploy --url URL --token TOKEN [flags]   migrate apps from a Dokploy instance

Run "%[1]s migrate <source> -h" for a source's own flags.
`, prog)
}
