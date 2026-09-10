package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// emailJSONUsage is every email subcommand's --json flag description:
// identical across get/set since each returns the same settings shape.
const emailJSONUsage = "print the settings as JSON to stdout and nothing else"

// runEmail dispatches "email <verb> [flags]" to one of get/set, the same
// platform-singleton-settings dispatch shape as runRegistry and
// runCloudflareTunnel. Not nested under a "settings" parent command: no
// such grouping exists anywhere else in this CLI.
func runEmail(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, emailUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, emailUsage(prog))
		return exitOK
	case "get":
		return runEmailGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runEmailSet(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown email subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, emailUsage(prog))
		return exitUsage
	}
}

func emailUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s email get [flags]                                        show the current outbound email settings
  %[1]s email set --backend smtp|ses [flags]                    configure the outbound email backend

Configures the control plane's outbound email (invite emails, alert
notifications) via SMTP or Amazon SES. --backend "" disables outbound
email. Credential fields (--smtp-password, --ses-secret-access-key) are
write-only: never echoed back by "get", only smtp_password_set/
ses_secret_access_key_set booleans report whether one is stored. "set" is
a full replace of every field for the chosen backend, so a headless
bootstrap script must pass every flag it wants kept on every call, e.g.:

  %[1]s email set --backend smtp --smtp-host smtp.example.com \
    --smtp-port 587 --smtp-username bot --smtp-from noreply@example.com \
    --smtp-password hunter2

Run "%[1]s email <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runEmailGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "email get", emailJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s email get [flags]\n\nShows the current outbound email settings.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	settings, err := client.GetEmailSettings(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get email settings: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, settings, func() { printEmailSettings(stdout, settings) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printEmailSettings(out io.Writer, s emailSettingsResource) {
	backend := s.Backend
	if backend == "" {
		backend = "(disabled)"
	}
	_, _ = fmt.Fprintf(out, "backend:                     %s\n", backend)
	if s.Backend == apiclient.EmailBackendSMTP {
		_, _ = fmt.Fprintf(out, "smtp_host:                   %s\n", s.SMTPHost)
		_, _ = fmt.Fprintf(out, "smtp_port:                   %d\n", s.SMTPPort)
		_, _ = fmt.Fprintf(out, "smtp_username:               %s\n", s.SMTPUsername)
		_, _ = fmt.Fprintf(out, "smtp_from:                   %s\n", s.SMTPFrom)
		_, _ = fmt.Fprintf(out, "smtp_password_set:           %v\n", s.SMTPPasswordSet)
	}
	if s.Backend == apiclient.EmailBackendSES {
		_, _ = fmt.Fprintf(out, "ses_region:                  %s\n", s.SESRegion)
		_, _ = fmt.Fprintf(out, "ses_access_key_id:           %s\n", s.SESAccessKeyID)
		_, _ = fmt.Fprintf(out, "ses_from:                    %s\n", s.SESFrom)
		_, _ = fmt.Fprintf(out, "ses_secret_access_key_set:   %v\n", s.SESSecretAccessKeySet)
	}
}

func runEmailSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "email set", emailJSONUsage, stderr)
	var backend, smtpHost, smtpUsername, smtpFrom, smtpPassword string
	var smtpPort int
	var sesRegion, sesAccessKeyID, sesFrom, sesSecretAccessKey string
	fs.StringVar(&backend, "backend", "", `email backend: "smtp", "ses", or "" to disable outbound email`)
	fs.StringVar(&smtpHost, "smtp-host", "", "SMTP server hostname (required when --backend=smtp)")
	fs.IntVar(&smtpPort, "smtp-port", 0, "SMTP server port (required when --backend=smtp)")
	fs.StringVar(&smtpUsername, "smtp-username", "", "SMTP username (optional)")
	fs.StringVar(&smtpFrom, "smtp-from", "", "SMTP \"from\" address (required when --backend=smtp)")
	fs.StringVar(&smtpPassword, "smtp-password", "", "SMTP password; omit to keep the currently stored one")
	fs.StringVar(&sesRegion, "ses-region", "", "AWS SES region, e.g. us-east-1 (required when --backend=ses)")
	fs.StringVar(&sesAccessKeyID, "ses-access-key-id", "", "AWS access key ID (required when --backend=ses)")
	fs.StringVar(&sesFrom, "ses-from", "", "SES \"from\" address (required when --backend=ses)")
	fs.StringVar(&sesSecretAccessKey, "ses-secret-access-key", "", "AWS secret access key; omit to keep the currently stored one")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s email set --backend smtp|ses [flags]\n\nConfigures outbound email, a full replace of every field for the chosen\nbackend. Omitting a password/secret flag keeps whatever is already\nstored.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	settings, err := client.UpdateEmailSettings(context.Background(), emailSettingsResource{
		Backend:            backend,
		SMTPHost:           smtpHost,
		SMTPPort:           smtpPort,
		SMTPUsername:       smtpUsername,
		SMTPFrom:           smtpFrom,
		SMTPPassword:       smtpPassword,
		SESRegion:          sesRegion,
		SESAccessKeyID:     sesAccessKeyID,
		SESFrom:            sesFrom,
		SESSecretAccessKey: sesSecretAccessKey,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set email settings: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, settings, func() { printEmailSettings(stdout, settings) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
