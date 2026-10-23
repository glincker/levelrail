package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func settingsEmailBrandingUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s settings email branding get [flags]
  %[1]s settings email branding set [branding flags]
  %[1]s settings email branding preview [branding flags]

Branding is shared by every rich email. The logo is an https URL that the
server never fetches; a remote logo tells its host when a message is opened.
The footer is plain text plus https links (no HTML). Footer text accepts the
global variables such as {{brand_name}} and {{year}}.

Branding flags (only the ones you pass change; the rest keep their saved value):
  --logo-url URL            https logo, or "" to clear
  --brand-color HEX         accent color such as #107292, or "" for the brand default
  --footer-text TEXT        footer message
  --footer-address TEXT     postal address line
  --footer-link "Label|URL" repeatable https footer link (replaces the saved list)
  --clear-footer-links      remove every footer link
  --powered-by[=false]      show or hide the "Powered by" line

"preview" renders the result around a sample message without saving it and
prints the HTML (or the full response with --json).
`, prog)
}

// runSettingsEmailBranding dispatches "settings email branding get|set|preview".
func runSettingsEmailBranding(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, settingsEmailBrandingUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, settingsEmailBrandingUsage(prog))
		return exitOK
	case "get":
		return runListCommand(prog, args[1:], stdout, stderr, lookupEnv, listCommandParams[apiclient.EmailBranding]{
			cmdLabel:  "settings email branding get",
			jsonUsage: "print the branding as JSON to stdout and nothing else",
			usageText: fmt.Sprintf("Usage:\n  %s settings email branding get [flags]\n\nShows the logo, accent color and footer every rich email uses.\n\nFlags:\n", prog),
			fetch:     func(c *Client, ctx context.Context) (apiclient.EmailBranding, error) { return c.GetEmailBranding(ctx) },
			errVerb:   "get email branding",
			print:     printEmailBrandingHuman,
		})
	case "set":
		return runSettingsEmailBrandingWrite(prog, "set", args[1:], stdout, stderr, lookupEnv)
	case "preview":
		return runSettingsEmailBrandingWrite(prog, "preview", args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown settings email branding subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, settingsEmailBrandingUsage(prog))
		return exitUsage
	}
}

func runSettingsEmailBrandingWrite(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "settings email branding " + verb
	fs, tokenP, apiURLP, profileP, jsonOutP, outputP, queryP := apiFlagSet(prog, label, "print the result as JSON to stdout and nothing else", stderr)
	var logo, color, footerText, footerAddress string
	var links stringListFlag
	var clearLinks, poweredBy bool
	fs.StringVar(&logo, "logo-url", "", "https logo URL, or \"\" to clear")
	fs.StringVar(&color, "brand-color", "", "accent color such as #107292, or \"\" for the brand default")
	fs.StringVar(&footerText, "footer-text", "", "footer message")
	fs.StringVar(&footerAddress, "footer-address", "", "postal address line")
	fs.Var(&links, "footer-link", "footer link as \"Label|https://url\" (repeatable)")
	fs.BoolVar(&clearLinks, "clear-footer-links", false, "remove every footer link")
	fs.BoolVar(&poweredBy, "powered-by", false, "show the \"Powered by\" line")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, settingsEmailBrandingUsage(prog))
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonOutP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	cur, err := client.GetEmailBranding(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get email branding: %w", err))
	}
	if set["logo-url"] {
		cur.LogoURL = logo
	}
	if set["brand-color"] {
		cur.BrandColor = color
	}
	if set["footer-text"] {
		cur.FooterText = footerText
	}
	if set["footer-address"] {
		cur.FooterAddress = footerAddress
	}
	if set["powered-by"] {
		cur.ShowPoweredBy = poweredBy
	}
	if clearLinks {
		cur.FooterLinks = []apiclient.EmailFooterLink{}
	}
	if len(links) > 0 {
		cur.FooterLinks = cur.FooterLinks[:0:0]
		for _, l := range links {
			name, url, found := strings.Cut(l, "|")
			if !found {
				return reportError(stdout, stderr, jsonOut, fmt.Errorf("--footer-link %q must look like \"Label|https://url\"", l))
			}
			cur.FooterLinks = append(cur.FooterLinks, apiclient.EmailFooterLink{Label: strings.TrimSpace(name), URL: strings.TrimSpace(url)})
		}
	}

	if verb == "preview" {
		prev, err := client.PreviewEmailBranding(context.Background(), cur)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("preview email branding: %w", err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, prev, func() { _, _ = fmt.Fprintln(stdout, prev.HTML) })
	}
	saved, err := client.UpdateEmailBranding(context.Background(), cur)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set email branding: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, saved, func() { printEmailBrandingHuman(stdout, saved) })
}

func printEmailBrandingHuman(out io.Writer, b apiclient.EmailBranding) {
	_, _ = fmt.Fprintf(out, "logo_url:        %s\n", b.LogoURL)
	_, _ = fmt.Fprintf(out, "brand_color:     %s\n", b.BrandColor)
	_, _ = fmt.Fprintf(out, "footer_text:     %s\n", strings.ReplaceAll(b.FooterText, "\n", " / "))
	_, _ = fmt.Fprintf(out, "footer_address:  %s\n", strings.ReplaceAll(b.FooterAddress, "\n", " / "))
	for _, l := range b.FooterLinks {
		_, _ = fmt.Fprintf(out, "footer_link:     %s -> %s\n", l.Label, l.URL)
	}
	_, _ = fmt.Fprintf(out, "show_powered_by: %v\n", b.ShowPoweredBy)
}
