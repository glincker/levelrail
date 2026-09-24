package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const (
	attentionCritical = "critical"
	attentionWarning  = "warning"
)

type attentionItem struct {
	Severity string `json:"severity"`
	Kind     string `json:"kind"`
	Subject  string `json:"subject"`
	Detail   string `json:"detail"`
}

const (
	diskWarnFreePercent     = 10.0
	diskCriticalFreePercent = 5.0
)

// attentionInput is everything buildAttentionItems reads; any field may be
// zero when its endpoint was unavailable.
type attentionInput struct {
	Apps   []apiclient.AppStatusEntry
	Nodes  []nodeResource
	Certs  []apiclient.CertificateResource
	Doctor systemDoctorResource
	Failed []apiclient.FailedDeployResource
	Status apiclient.SystemStatusResource
}

// diskAttentionItem mirrors web/src/lib/diskPressure.ts: warn below 10
// percent free, critical below 5 percent.
func diskAttentionItem(s apiclient.SystemStatusResource) (attentionItem, bool) {
	if s.DataDirTotalBytes <= 0 {
		return attentionItem{}, false
	}
	pct := float64(s.DataDirFreeBytes) / float64(s.DataDirTotalBytes) * 100
	detail := fmt.Sprintf("%.1f%% free (%d of %d bytes)", pct, s.DataDirFreeBytes, s.DataDirTotalBytes)
	switch {
	case pct < diskCriticalFreePercent:
		return attentionItem{attentionCritical, "disk", "data dir", detail}, true
	case pct < diskWarnFreePercent:
		return attentionItem{attentionWarning, "disk", "data dir", detail}, true
	}
	return attentionItem{}, false
}

// buildAttentionItems mirrors web/src/lib/attention.ts: disk pressure,
// failing apps, recent failed deploys, offline nodes, bad certificates,
// and doctor warnings or failures, critical first.
func buildAttentionItems(in attentionInput) []attentionItem {
	apps, nodes, certs, doctor := in.Apps, in.Nodes, in.Certs, in.Doctor
	items := []attentionItem{}
	if it, ok := diskAttentionItem(in.Status); ok {
		items = append(items, it)
	}
	for _, f := range in.Failed {
		detail := f.Error
		if detail == "" {
			detail = "deploy failed"
		}
		items = append(items, attentionItem{attentionCritical, "deploy", f.ServiceName, detail})
	}
	for _, a := range apps {
		if a.Status.Variant == "destructive" {
			items = append(items, attentionItem{attentionCritical, "app", a.Name, a.Status.Label})
		}
	}
	for _, n := range nodes {
		if n.Status == "offline" {
			detail := "never reported in"
			if n.LastSeenAt != nil {
				detail = "last seen " + n.LastSeenAt.Format("2006-01-02 15:04 MST")
			}
			items = append(items, attentionItem{attentionCritical, "node", n.Name, detail})
		}
	}
	for _, c := range certs {
		switch c.Status {
		case "expired":
			items = append(items, attentionItem{attentionCritical, "certificate", c.Domain, "expired " + c.NotAfter.Format("2006-01-02")})
		case "expiring_soon":
			items = append(items, attentionItem{attentionWarning, "certificate", c.Domain, "expires " + c.NotAfter.Format("2006-01-02")})
		}
	}
	for _, c := range doctor.Checks {
		switch c.Status {
		case "fail":
			items = append(items, attentionItem{attentionCritical, "doctor", c.Name, c.Message})
		case "warn":
			items = append(items, attentionItem{attentionWarning, "doctor", c.Name, c.Message})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Severity == attentionCritical && items[j].Severity != attentionCritical
	})
	return items
}

// runAttention implements "attention": the CLI side of the dashboard's
// Status page. Exit code is 1 when any item is critical.
func runAttention(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "attention", "print attention items as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, attentionUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	apps, err := client.ListAppStatuses(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list apps: %w", err))
	}
	nodes, err := client.ListNodes(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list nodes: %w", err))
	}
	certs, err := client.ListCertificates(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list certificates: %w", err))
	}
	doctor, err := client.GetSystemDoctor(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get system doctor report: %w", err))
	}

	// Disk and failed deploys are optional sources: a failure drops only
	// its own items, matching the dashboard.
	failed, _ := client.ListFailedDeploys(ctx, "24h")
	status, _ := client.GetSystemStatus(ctx)

	items := buildAttentionItems(attentionInput{Apps: apps, Nodes: nodes, Certs: certs, Doctor: doctor, Failed: failed, Status: status})
	if err := renderResult(stdout, of.Format, of.Query, items, func() { printAttentionHuman(stdout, items) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	for _, it := range items {
		if it.Severity == attentionCritical {
			return exitUsage
		}
	}
	return exitOK
}

func printAttentionHuman(out io.Writer, items []attentionItem) {
	if len(items) == 0 {
		_, _ = fmt.Fprintln(out, "All systems healthy.")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SEVERITY\tKIND\tSUBJECT\tDETAIL")
	for _, it := range items {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", it.Severity, it.Kind, it.Subject, it.Detail)
	}
	_ = tw.Flush()
}

func attentionUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s attention [flags]

Lists everything that needs attention right now: failing apps, failed
deploys from the last 24 hours, low disk space (warn under 10 percent free,
critical under 5), offline nodes, expired or expiring certificates, and
doctor warnings or failures.
Exit code is 1 if any item is critical, 0 otherwise.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print attention items as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
