package main

import (
	"context"
	"fmt"
	"io"
)

// runSettingsEmail dispatches "settings email <verb> [flags]" to one of
// get/set, the singleton outbound-email (SMTP or SES) configuration
// (internal/api/email_settings.go).
func runSettingsEmail(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, settingsEmailUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, settingsEmailUsage(prog))
		return exitOK
	case "get":
		return runSettingsEmailGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runSettingsEmailSet(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown settings email subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, settingsEmailUsage(prog))
		return exitUsage
	}
}

func settingsEmailUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s settings email get [flags]
  %[1]s settings email set --backend smtp|ses [flags]

Configures outbound email, used for invite/notification delivery. Backend
is "smtp", "ses", or "" (unconfigured). SMTP requires --smtp-host,
--smtp-port, and --smtp-from; SES requires --ses-region, --ses-access-key-id,
and --ses-from. Credential flags may be omitted on an update to leave the
currently stored value unchanged.

Run "%[1]s settings email <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runSettingsEmailGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings email get", "print the email settings as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings email get [flags]\n\nShows the current outbound email settings. Credential values are never\nreturned, only whether one is stored.\n\nFlags:\n", prog)
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

	return writeScheduledTaskResult(stdout, stderr, of, settings, func() { printEmailSettingsHuman(stdout, settings) })
}

func runSettingsEmailSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings email set", "print the updated email settings as JSON to stdout and nothing else", stderr)
	var backend, smtpHost, smtpUsername, smtpFrom, smtpPassword string
	var smtpPort int
	var sesRegion, sesAccessKeyID, sesFrom, sesSecretAccessKey string
	fs.StringVar(&backend, "backend", "", "email backend: \"smtp\", \"ses\", or \"\" to unconfigure")
	fs.StringVar(&smtpHost, "smtp-host", "", "SMTP server hostname (required when backend is smtp)")
	fs.IntVar(&smtpPort, "smtp-port", 0, "SMTP server port (required when backend is smtp)")
	fs.StringVar(&smtpUsername, "smtp-username", "", "SMTP username")
	fs.StringVar(&smtpFrom, "smtp-from", "", "SMTP \"from\" address (required when backend is smtp)")
	fs.StringVar(&smtpPassword, "smtp-password", "", "SMTP password (omit on update to keep the stored value)")
	fs.StringVar(&sesRegion, "ses-region", "", "AWS SES region (required when backend is ses)")
	fs.StringVar(&sesAccessKeyID, "ses-access-key-id", "", "AWS SES access key ID (required when backend is ses)")
	fs.StringVar(&sesFrom, "ses-from", "", "AWS SES \"from\" address (required when backend is ses)")
	fs.StringVar(&sesSecretAccessKey, "ses-secret-access-key", "", "AWS SES secret access key (omit on update to keep the stored value)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings email set --backend smtp|ses [flags]\n\nConfigures outbound email.\n\nFlags:\n", prog)
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

	return writeScheduledTaskResult(stdout, stderr, of, settings, func() { printEmailSettingsHuman(stdout, settings) })
}

func printEmailSettingsHuman(out io.Writer, s emailSettingsResource) {
	_, _ = fmt.Fprintf(out, "backend:                  %s\n", s.Backend)
	if s.Backend == "smtp" {
		_, _ = fmt.Fprintf(out, "smtp_host:                %s\n", s.SMTPHost)
		_, _ = fmt.Fprintf(out, "smtp_port:                %d\n", s.SMTPPort)
		_, _ = fmt.Fprintf(out, "smtp_username:            %s\n", s.SMTPUsername)
		_, _ = fmt.Fprintf(out, "smtp_from:                %s\n", s.SMTPFrom)
		_, _ = fmt.Fprintf(out, "smtp_password_set:        %v\n", s.SMTPPasswordSet)
	}
	if s.Backend == "ses" {
		_, _ = fmt.Fprintf(out, "ses_region:               %s\n", s.SESRegion)
		_, _ = fmt.Fprintf(out, "ses_access_key_id:        %s\n", s.SESAccessKeyID)
		_, _ = fmt.Fprintf(out, "ses_from:                 %s\n", s.SESFrom)
		_, _ = fmt.Fprintf(out, "ses_secret_access_key_set: %v\n", s.SESSecretAccessKeySet)
	}
}
