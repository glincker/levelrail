package main

import (
	"context"
	"fmt"
	"io"
)

// runDatabasesCredentials implements "databases credentials <name>": GET
// /api/v1/databases/{name}/credentials (internal/api/database_credentials.go's
// own handleGetDatabaseCredentials). AbilityReadSensitive-gated
// server-side: this discloses a real secret's plaintext, so it sits one
// tier above "databases get".
func runDatabasesCredentials(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases credentials", "print the credentials as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesCredentialsUsage(prog)) }

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "databases credentials", "database name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	creds, err := client.GetDatabaseCredentials(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get credentials for database %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, creds, func() {
		_, _ = fmt.Fprintf(stdout, "host:     %s\n", creds.Host)
		_, _ = fmt.Fprintf(stdout, "port:     %d\n", creds.Port)
		if creds.Database != "" {
			_, _ = fmt.Fprintf(stdout, "database: %s\n", creds.Database)
		}
		if creds.Username != "" {
			_, _ = fmt.Fprintf(stdout, "username: %s\n", creds.Username)
		}
		if creds.Password != "" {
			_, _ = fmt.Fprintf(stdout, "password: %s\n", creds.Password)
		}
		_, _ = fmt.Fprintf(stdout, "url:      %s\n", creds.URL)
	})
}

func databasesCredentialsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases credentials <name> [flags]

Shows the host, port, username, password, and full connection URL for a
managed database, for plugging it into an external client (a GUI
database tool, a script running elsewhere).

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the credentials as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
