package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const mailCertUsageLines = `  %[1]s mail cert status [--app name]            show the served certificate: subject, issuer, expiry, days left, self-signed or not
  %[1]s mail cert upload --cert F --key F [--app name] [--restart-now]   install your own certificate (asks your password)
  %[1]s mail cert obtain [--app name]             get one through ACME (not yet implemented)
`

// runMailCert handles "mail cert status|upload|obtain".
func runMailCert(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || (args[0] != "status" && args[0] != "upload" && args[0] != "obtain") {
		_, _ = fmt.Fprintf(stderr, "Usage:\n"+mailCertUsageLines, prog)
		return exitUsage
	}
	sub, rest := args[0], args[1:]
	name := "mail cert " + sub
	var app, certFile, keyFile string
	var restartNow bool
	usage := name + " [--app name]"
	if sub == "upload" {
		usage = name + " --cert chain.pem --key key.pem [--app name] [--restart-now]"
	}
	c, code, ok := parseDBAccess(prog, name, usage, rest, 0, "", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&app, "app", "", "mail app when several are installed")
		if sub == "upload" {
			fs.StringVar(&certFile, "cert", "", "PEM file with the certificate chain, leaf first, covering the mail host name")
			fs.StringVar(&keyFile, "key", "", "PEM file with the unencrypted private key; it is sealed at rest and never shown again")
			fs.BoolVar(&restartNow, "restart-now", false, "restart the mail server now even outside APP_MAIL_RESTART_WINDOW")
		}
	})
	if !ok {
		return code
	}
	if sub == "obtain" {
		if err := c.client.ObtainMailCert(context.Background(), app); err != nil {
			return c.fail(fmt.Errorf("mail cert obtain: %w", err))
		}
		return 0
	}
	if sub == "upload" {
		if certFile == "" || keyFile == "" {
			return c.fail(fmt.Errorf("mail cert upload: --cert and --key are both required"))
		}
		certPEM, err := os.ReadFile(certFile) //nolint:gosec // the operator names the file
		if err != nil {
			return c.fail(fmt.Errorf("mail cert upload: read --cert: %w", err))
		}
		keyPEM, err := os.ReadFile(keyFile) //nolint:gosec // the operator names the file
		if err != nil {
			return c.fail(fmt.Errorf("mail cert upload: read --key: %w", err))
		}
		res, err := withStepUp(stderr, func(p *apiclient.StepUpProof) (apiclient.MailCertUploadResult, error) {
			return c.client.UploadMailCert(context.Background(), app, string(certPEM), string(keyPEM), restartNow, p)
		})
		if err != nil {
			return c.fail(fmt.Errorf("mail cert upload: %w", err))
		}
		return c.result(res, func() {
			_, _ = fmt.Fprintf(stdout, "app: %s\noutcome: %s\nserial: %s\nsha256: %s\nnot after: %s (%d days)\n%s\n", res.App, res.Outcome, res.Serial, res.SHA256, res.NotAfter, res.DaysLeft, res.Message)
		})
	}
	st, err := c.client.MailCertStatus(context.Background(), app)
	if err != nil {
		return c.fail(fmt.Errorf("mail cert status: %w", err))
	}
	return c.result(st, func() {
		_, _ = fmt.Fprintf(stdout, "app: %s\nmail host: %s\nsource: %s\nrestart window: %s\n", st.App, st.Hostname, st.Source, st.RestartWindow)
		if !st.Served {
			_, _ = fmt.Fprintf(stdout, "served: no (%s)\nnext: %s\n", st.ProbeError, st.NextStep)
			return
		}
		_, _ = fmt.Fprintf(stdout, "subject: %s\nissuer: %s\nnot after: %s\ndays left: %d\nself-signed: %t\nserial: %s\nsha256: %s\nnext: %s\n",
			st.Subject, st.Issuer, st.NotAfter, st.DaysLeft, st.SelfSigned, st.Serial, st.SHA256, st.NextStep)
	})
}
