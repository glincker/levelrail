package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"golang.org/x/term"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func settingsEmailProvidersUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s settings email providers list [flags]
  %[1]s settings email providers add --backend smtp|ses|ses_smtp|resend|postmark|sendgrid|mailgun [flags]
  %[1]s settings email providers set <id> [flags]
  %[1]s settings email providers rm <id> [flags]
  %[1]s settings email providers test <id> [--to ADDRESS] [flags]
  %[1]s settings email providers promote <id> [flags]
  %[1]s settings email providers reveal <id> <field> --yes-print [flags]

One primary provider plus at most one backup. The first provider added is the
primary, the second the backup. When the primary fails before a message is
accepted, sending falls over to the backup. "promote" swaps the two roles.

Credentials are never passed as arguments. "add" prompts for the credential when
stdin is a terminal; "--secret-stdin" reads it from the first line of stdin, and
"--prompt-secret" forces the prompt. On "set", the stored credential is kept unless
one of those flags is given. The credential that applies is the one for the backend
(SMTP password, SES secret access key, API key or token).

"reveal" prints one stored credential after asking for your password and
authenticator code at the terminal. It needs two-factor authentication on your
account, is rate limited and is audited. It refuses unless --yes-print is given,
because the value goes to stdout.

Run "%[1]s settings email providers <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runSettingsEmailProviders(prog string, args []string, stdin io.Reader, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, settingsEmailProvidersUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, settingsEmailProvidersUsage(prog))
		return exitOK
	case "list":
		return runListCommand(prog, args[1:], stdout, stderr, lookupEnv, listCommandParams[apiclient.EmailProviderList]{
			cmdLabel:  "settings email providers list",
			jsonUsage: "print the providers as JSON to stdout and nothing else",
			usageText: fmt.Sprintf("Usage:\n  %s settings email providers list [flags]\n\nLists the primary and backup email providers. Credential values are never\nreturned, only whether one is stored.\n\nFlags:\n", prog),
			fetch: func(c *Client, ctx context.Context) (apiclient.EmailProviderList, error) {
				return c.ListEmailProviders(ctx)
			},
			errVerb: "list email providers",
			print:   printEmailProviderList,
		})
	case "add":
		return runEmailProviderWrite(prog, args[1:], stdin, stdout, stderr, lookupEnv, true)
	case "set":
		return runEmailProviderWrite(prog, args[1:], stdin, stdout, stderr, lookupEnv, false)
	case "rm":
		return runEmailProviderRm(prog, args[1:], stdout, stderr, lookupEnv)
	case "test":
		return runEmailProviderTest(prog, args[1:], stdout, stderr, lookupEnv)
	case "promote":
		return runEmailProviderPromote(prog, args[1:], stdout, stderr, lookupEnv)
	case "reveal":
		return runEmailProviderReveal(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown settings email providers subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, settingsEmailProvidersUsage(prog))
		return exitUsage
	}
}

// emailProviderFlags are the non-secret provider flags shared by add and set.
type emailProviderFlags struct {
	name, backend, from, tlsMode                      string
	smtpHost, smtpUsername                            string
	sesRegion, sesAccessKeyID, sesSMTPUsername        string
	mailgunDomain, mailgunRegion                      string
	smtpPort, sesSMTPPort                             int
	tlsSkipVerify, enabled, secretStdin, promptSecret bool
}

func (f *emailProviderFlags) bind(fs *flag.FlagSet) {
	fs.StringVar(&f.name, "name", "", "display name for this provider (default: the backend)")
	fs.StringVar(&f.backend, "backend", "", "smtp, ses, ses_smtp, resend, postmark, sendgrid or mailgun")
	fs.BoolVar(&f.enabled, "enabled", true, "send through this provider; --enabled=false keeps it configured but idle")
	fs.StringVar(&f.from, "from", "", "provider sender address (smtp, ses, ses_smtp and resend; other backends use the shared sender)")
	fs.StringVar(&f.tlsMode, "tls-mode", "", "SMTP TLS: \"\" (auto), starttls, tls or none")
	fs.BoolVar(&f.tlsSkipVerify, "tls-skip-verify", false, "do not verify the SMTP server certificate")
	fs.StringVar(&f.smtpHost, "smtp-host", "", "SMTP server hostname")
	fs.IntVar(&f.smtpPort, "smtp-port", 0, "SMTP server port")
	fs.StringVar(&f.smtpUsername, "smtp-username", "", "SMTP username")
	fs.StringVar(&f.sesRegion, "ses-region", "", "AWS SES region such as us-east-1")
	fs.StringVar(&f.sesAccessKeyID, "ses-access-key-id", "", "AWS SES access key ID")
	fs.StringVar(&f.sesSMTPUsername, "ses-smtp-username", "", "SES SMTP username (backend ses_smtp)")
	fs.IntVar(&f.sesSMTPPort, "ses-smtp-port", 0, "SES SMTP port: 587 (default), 465, 25, 2587 or 2465")
	fs.StringVar(&f.mailgunDomain, "mailgun-domain", "", "Mailgun sending domain")
	fs.StringVar(&f.mailgunRegion, "mailgun-region", "", "Mailgun region: us (default) or eu")
	fs.BoolVar(&f.secretStdin, "secret-stdin", false, "read the credential from the first line of stdin")
	fs.BoolVar(&f.promptSecret, "prompt-secret", false, "prompt for the credential without echo")
}

var errProviderFromUnsupported = errors.New("--from applies to smtp, ses, ses_smtp and resend; this backend uses the shared sender address (settings email set --from)")

// apply overlays the flags the user actually passed onto p, so set leaves everything else as stored.
func (f *emailProviderFlags) apply(fs *flag.FlagSet, p *apiclient.EmailProvider) error {
	fs.Visit(func(fl *flag.Flag) {
		switch fl.Name {
		case "name":
			p.Name = f.name
		case "backend":
			p.Backend = f.backend
		case "enabled":
			v := f.enabled
			p.Enabled = &v
		case "tls-mode":
			p.TLSMode = f.tlsMode
		case "tls-skip-verify":
			p.TLSSkipVerify = f.tlsSkipVerify
		case "smtp-host":
			p.SMTPHost = f.smtpHost
		case "smtp-port":
			p.SMTPPort = f.smtpPort
		case "smtp-username":
			p.SMTPUsername = f.smtpUsername
		case "ses-region":
			p.SESRegion = f.sesRegion
		case "ses-access-key-id":
			p.SESAccessKeyID = f.sesAccessKeyID
		case "ses-smtp-username":
			p.SESSMTPUsername = f.sesSMTPUsername
		case "ses-smtp-port":
			p.SESSMTPPort = f.sesSMTPPort
		case "mailgun-domain":
			p.MailgunDomain = f.mailgunDomain
		case "mailgun-region":
			p.MailgunRegion = f.mailgunRegion
		}
	})
	visited := false
	fs.Visit(func(fl *flag.Flag) { visited = visited || fl.Name == "from" })
	if !visited {
		return nil
	}
	switch p.Backend {
	case "smtp":
		p.SMTPFrom = f.from
	case "ses", "ses_smtp":
		p.SESFrom = f.from
	case "resend":
		p.ResendFrom = f.from
	default:
		return errProviderFromUnsupported
	}
	return nil
}

func setProviderSecret(p *apiclient.EmailProvider, secret string) {
	switch p.Backend {
	case "smtp":
		p.SMTPPassword = secret
	case "ses":
		p.SESSecretAccessKey = secret
	case "ses_smtp":
		p.SESSMTPPassword = secret
	case "resend":
		p.ResendAPIKey = secret
	case "postmark":
		p.PostmarkServerToken = secret
	case "sendgrid":
		p.SendGridAPIKey = secret
	case "mailgun":
		p.MailgunAPIKey = secret
	}
}

// promptProviderSecret reads a credential from the terminal without echo; a variable so tests can stub it.
var promptProviderSecret = func(stderr io.Writer) (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", errors.New("--prompt-secret needs a terminal; use --secret-stdin to pipe the credential")
	}
	_, _ = fmt.Fprint(stderr, "Credential (input hidden): ")
	b, err := term.ReadPassword(fd)
	_, _ = fmt.Fprintln(stderr)
	return string(b), err
}

// readProviderSecret returns the credential from stdin or a prompt, or "" when the user supplied none.
func readProviderSecret(f *emailProviderFlags, promptByDefault bool, stdin io.Reader, stderr io.Writer) (string, error) {
	switch {
	case f.secretStdin:
		line, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("read credential from stdin: %w", err)
		}
		return strings.TrimRight(line, "\r\n"), nil
	case f.promptSecret || (promptByDefault && stdinIsTerminal()):
		return promptProviderSecret(stderr)
	}
	return "", nil
}

func runEmailProviderWrite(prog string, args []string, stdin io.Reader, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), isAdd bool) int {
	label := "settings email providers set"
	if isAdd {
		label = "settings email providers add"
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the provider as JSON to stdout and nothing else", stderr)
	var f emailProviderFlags
	f.bind(fs)
	fs.Usage = func() {
		verb := "set <id>"
		if isAdd {
			verb = "add --backend NAME"
		}
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings email providers %s [flags]\n\nOnly the flags you pass change; the credential is read from stdin or a prompt, never from arguments.\n\nFlags:\n", prog, verb)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	var id string
	var base apiclient.EmailProvider
	if isAdd {
		if len(fs.Args()) != 0 {
			_, _ = fmt.Fprintf(stderr, "%s: %s takes flags only\n", prog, label)
			return exitUsage
		}
	} else {
		id, ok = requireOneArg(fs, stderr, prog, label, "provider id")
		if !ok {
			return exitUsage
		}
		existing, err := client.GetEmailProvider(ctx, id)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("get email provider %q: %w", id, err))
		}
		base = existing
	}
	if err := f.apply(fs, &base); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, err)
		return exitValidation
	}
	if isAdd && base.Backend == "" {
		_, _ = fmt.Fprintf(stderr, "%s: --backend is required\n", prog)
		return exitValidation
	}
	secret, err := readProviderSecret(&f, isAdd, stdin, stderr)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, err)
		return exitValidation
	}
	setProviderSecret(&base, secret)

	var saved apiclient.EmailProvider
	if isAdd {
		saved, err = client.CreateEmailProvider(ctx, base)
	} else {
		saved, err = client.UpdateEmailProvider(ctx, id, base)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s: %w", label, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, saved, func() { printEmailProvider(stdout, saved) })
}

func runEmailProviderRm(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings email providers rm", "print {\"deleted\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings email providers rm <id> [flags]\n\nRemoves a provider and its stored credential. The primary cannot be removed while a backup exists.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "settings email providers rm", "provider id")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.DeleteEmailProvider(context.Background(), id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("remove email provider %q: %w", id, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]bool{"deleted": true}, func() {
		_, _ = fmt.Fprintf(stdout, "email provider %q removed\n", id)
	})
}

func runEmailProviderTest(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings email providers test", "print the result as JSON to stdout and nothing else", stderr)
	to := fs.String("to", "", "recipient (default: the signed-in user's address)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings email providers test <id> [--to ADDRESS] [flags]\n\nSends one real message through that provider alone, with no failover.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "settings email providers test", "provider id")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.TestEmailProvider(context.Background(), id, *to)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("test email provider %q: %w", id, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintf(stdout, "sent to %s through %s (%s)\n", res.To, id, res.Provider)
	})
}

func runEmailProviderPromote(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings email providers promote", "print the providers as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings email providers promote <id> [flags]\n\nMakes the backup the primary and the primary the backup. Credentials follow the roles.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "settings email providers promote", "provider id")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	list, err := client.PromoteEmailProvider(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("promote email provider %q: %w", id, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, list, func() { printEmailProviderList(stdout, list) })
}

func runEmailProviderReveal(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	const label = "settings email providers reveal"
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the value as JSON to stdout and nothing else", stderr)
	yesPrint := fs.Bool("yes-print", false, "confirm that the credential may be printed to stdout")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s %s <id> <field> --yes-print [flags]\n\nPrints one stored credential (field is an envKey such as resend_api_key or smtp_password). Asks for your\npassword and authenticator code at the terminal, never from arguments or the environment.\n\nFlags:\n", prog, label)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, label, "a provider id and a credential field", 2)
	if !ok {
		return exitUsage
	}
	if !*yesPrint {
		_, _ = fmt.Fprintf(stderr, "%s: refusing to print a credential without --yes-print\n", prog)
		return exitValidation
	}
	id, field := rest[0], rest[1]
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := withStepUp(stderr, func(p *apiclient.StepUpProof) (apiclient.EmailSecretRevealed, error) {
		return client.RevealEmailSecret(context.Background(), id, field, p)
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("reveal %s of email provider %q: %w", field, id, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { _, _ = fmt.Fprintln(stdout, res.Value) })
}

func printEmailProviderList(out io.Writer, l apiclient.EmailProviderList) {
	if len(l.Providers) == 0 {
		_, _ = fmt.Fprintln(out, "no email providers configured")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tROLE\tBACKEND\tNAME\tENABLED\tCREDENTIAL\tLAST OK")
	for _, p := range l.Providers {
		lastOK := "-"
		if p.LastOKAt != nil {
			lastOK = p.LastOKAt.UTC().Format("2006-01-02 15:04")
		}
		cred := "missing"
		if p.SecretSet() {
			cred = "set"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%v\t%s\t%s\n", p.ID, p.Role, p.Backend, p.Name, p.Enabled == nil || *p.Enabled, cred, lastOK)
	}
	_ = tw.Flush()
}

func printEmailProvider(out io.Writer, p apiclient.EmailProvider) {
	printEmailProviderList(out, apiclient.EmailProviderList{Providers: []apiclient.EmailProvider{p}})
}
