package main

import (
	"context"
	"fmt"
	"io"
)

// runDoctor implements "doctor": GET /api/v1/system/doctor, an operator
// preflight bundle a superset of "status" (status.go) bundling several
// individual local health checks into one report.
func runDoctor(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "doctor", "print the doctor report as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, doctorUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	report, err := client.GetSystemDoctor(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get system doctor report: %w", err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, report); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printSystemDoctorHuman(stdout, report)

	if !report.OK {
		// doctor's own exit contract ("0 all ok, 1 any check failed") is
		// a report on the checks themselves, not a request-classification
		// code, so it reuses exitUsage's numeric value (1) as the
		// generic "not all clean" signal rather than adding a second,
		// unused-elsewhere constant for the same number.
		return exitUsage
	}
	return exitOK
}

func doctorUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s doctor [flags]

Runs a local preflight health check: Docker daemon reachability, disk
space and write access on the data directory, port 80/443 availability
for the embedded ingress, and control plane database reachability.

Exit code is 0 if every check is ok or warn, 1 if any check fails.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the doctor report as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
