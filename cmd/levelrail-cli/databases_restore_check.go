package main

import (
	"context"
	"fmt"
	"io"
	"time"
)

// runDatabasesRestoreTest implements "databases restore-test <name>": restore the newest
// backup into a throwaway container and report the result, never touching the live database.
func runDatabasesRestoreTest(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newProtectionCmd(prog, "databases restore-test", databasesRestoreTestUsage(prog), stdout, stderr, lookupEnv)
	var timeout time.Duration
	c.fs.DurationVar(&timeout, "timeout", 30*time.Minute, "how long to wait for the restore test")
	rest, exit, ok := c.parse(args, 1)
	if !ok {
		return exit
	}
	started, err := c.client.StartDatabaseRestoreTest(context.Background(), rest[0])
	if err != nil {
		return c.fail(fmt.Errorf("restore test %q: %w", rest[0], err))
	}
	return c.waitDrill(context.Background(), started.ID, timeout)
}

func databasesRestoreTestUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases restore-test <name> [--timeout D] [flags]

Restores the newest succeeded backup into a throwaway container, checks it,
removes it and records the result in the backup history. The live database is
never touched. Exits non-zero when the test fails.

Flags:
  --timeout duration     how long to wait for the restore test (default 30m0s)
  --token string         API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                 print the result as JSON to stdout, nothing else
  --output string        output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string         JMESPath expression to filter the result before printing
  -h, --help             show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}
