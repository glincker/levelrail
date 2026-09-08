package main

import (
	"context"
	"fmt"
	"io"
)

const settingsEmailJSONUsage = "print the settings as JSON to stdout and nothing else"

// runSettingsEmail dispatches "settings email <verb> [flags]" to one of
// get/set, mirroring runCloudflareTunnel's own get/set/disconnect
// dispatch shape for a single, always-present settings resource (no
// disconnect verb here: there is no PUT /api/v1/settings/email counterpart
// that clears the backend, only one that changes it).
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
  %[1]s settings email get [flags]                             show the current settings
  %[1]s settings email set --backend smtp|ses [flags]           configure outbound email

Configures the outbound email backend used for password resets, invites,
and anything else this control plane sends. Neither the SMTP password nor
the SES secret access key is ever returned by "get": only whether one is
currently stored.

Run "%[1]s settings email <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runSettingsEmailGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings email get", settingsEmailJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings email get [flags]\n\nShows the current outbound email settings.\n\nFlags:\n", prog)
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

	if err := renderResult(stdout, of.Format, of.Query, settings, func() { printEmailSettingsHuman(stdout, settings) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runSettingsEmailSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings email set", settingsEmailJSONUsage, stderr)
	var backendFlag, smtpHostFlag, smtpUsernameFlag, smtpFromFlag, smtpPasswordFlag string
	var smtpPortFlag int
	var sesRegionFlag, sesAccessKeyIDFlag, sesFromFlag, sesSecretAccessKeyFlag string
	fs.StringVar(&backendFlag, "backend", "", `email backend: "smtp" or "ses" (empty disables outbound email)`)
	fs.StringVar(&smtpHostFlag, "smtp-host", "", "SMTP server host (required when --backend=smtp)")
	fs.IntVar(&smtpPortFlag, "smtp-port", 0, "SMTP server port (required when --backend=smtp)")
	fs.StringVar(&smtpUsernameFlag, "smtp-username", "", "SMTP auth username")
	fs.StringVar(&smtpFromFlag, "smtp-from", "", "SMTP \"from\" address (required when --backend=smtp)")
	fs.StringVar(&smtpPasswordFlag, "smtp-password", "", "SMTP auth password (omit to keep the currently stored password)")
	fs.StringVar(&sesRegionFlag, "ses-region", "", "AWS SES region (required when --backend=ses)")
	fs.StringVar(&sesAccessKeyIDFlag, "ses-access-key-id", "", "AWS SES access key id (required when --backend=ses)")
	fs.StringVar(&sesFromFlag, "ses-from", "", "AWS SES \"from\" address (required when --backend=ses)")
	fs.StringVar(&sesSecretAccessKeyFlag, "ses-secret-access-key", "", "AWS SES secret access key (omit to keep the currently stored secret)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings email set --backend smtp|ses [flags]\n\nConfigures the outbound email backend.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	req := updateEmailSettingsRequest{
		Backend:            backendFlag,
		SMTPHost:           smtpHostFlag,
		SMTPPort:           smtpPortFlag,
		SMTPUsername:       smtpUsernameFlag,
		SMTPFrom:           smtpFromFlag,
		SMTPPassword:       smtpPasswordFlag,
		SESRegion:          sesRegionFlag,
		SESAccessKeyID:     sesAccessKeyIDFlag,
		SESFrom:            sesFromFlag,
		SESSecretAccessKey: sesSecretAccessKeyFlag,
	}
	settings, err := client.SetEmailSettings(context.Background(), req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set email settings: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, settings, func() { printEmailSettingsHuman(stdout, settings) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printEmailSettingsHuman(out io.Writer, s emailSettingsResource) {
	_, _ = fmt.Fprintf(out, "backend:                   %s\n", s.Backend)
	_, _ = fmt.Fprintf(out, "smtp_host:                 %s\n", s.SMTPHost)
	_, _ = fmt.Fprintf(out, "smtp_port:                 %d\n", s.SMTPPort)
	_, _ = fmt.Fprintf(out, "smtp_username:             %s\n", s.SMTPUsername)
	_, _ = fmt.Fprintf(out, "smtp_from:                 %s\n", s.SMTPFrom)
	_, _ = fmt.Fprintf(out, "smtp_password_set:         %v\n", s.SMTPPasswordSet)
	_, _ = fmt.Fprintf(out, "ses_region:                %s\n", s.SESRegion)
	_, _ = fmt.Fprintf(out, "ses_access_key_id:         %s\n", s.SESAccessKeyID)
	_, _ = fmt.Fprintf(out, "ses_from:                  %s\n", s.SESFrom)
	_, _ = fmt.Fprintf(out, "ses_secret_access_key_set: %v\n", s.SESSecretAccessKeySet)
}
