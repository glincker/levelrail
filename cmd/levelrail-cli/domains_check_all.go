package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const (
	checkAllWorkersDefault = 4
	checkAllTimeoutDefault = 30 * time.Second
)

// domainLiveRow is one domain's readiness summary.
type domainLiveRow struct {
	Domain      string `json:"domain"`
	App         string `json:"app,omitempty"`
	DNS         string `json:"dns"`
	Certificate string `json:"certificate"`
	Site        string `json:"site"`
	Next        string `json:"next"`
	Live        bool   `json:"live"`
}

func domainsCheckAllUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains check-all [--app <name>] [--workers N] [--timeout 30s] [flags]

Checks every domain (or one app's) and prints DNS, certificate, whether the site
answers, and the next step. Uses the same read-only check as "domains check --cutover".
The exit code is 1 when any domain is not fully live.
%[2]s`, prog, commonFlagsHelp)
}

func runDomainsCheckAll(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var appName string
	workers := checkAllWorkersDefault
	timeout := checkAllTimeoutDefault
	c, code, ok := parseCLICall(prog, "domains check-all", domainsCheckAllUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&appName, "app", "", "only check this app's domains")
		fs.IntVar(&workers, "workers", checkAllWorkersDefault, "domains checked at the same time")
		fs.DurationVar(&timeout, "timeout", checkAllTimeoutDefault, "time allowed per domain")
	})
	if !ok {
		return code
	}
	if workers < 1 {
		return c.invalid("--workers must be at least 1")
	}
	if timeout <= 0 {
		return c.invalid("--timeout must be positive")
	}
	domains, err := c.client.ListDomains(context.Background())
	if err != nil {
		return c.fail(fmt.Errorf("list domains: %w", err))
	}
	var targets []apiclient.DomainResource
	for _, d := range domains {
		if d.Automatic || (appName != "" && d.ServiceName != appName) {
			continue
		}
		targets = append(targets, d)
	}
	rows := checkDomainsLive(c.client, targets, workers, timeout)
	if code := c.render(rows, func() { printLiveRows(stdout, rows) }); code != exitOK {
		return code
	}
	for _, r := range rows {
		if !r.Live {
			return exitCheckFailed
		}
	}
	return exitOK
}

type domainVerifier interface {
	DomainVerify(ctx context.Context, domain string, expectIPs []string) (apiclient.DomainVerifyReport, error)
}

// checkDomainsLive runs the checks with a bounded worker pool; each call has its own deadline.
func checkDomainsLive(client domainVerifier, targets []apiclient.DomainResource, workers int, timeout time.Duration) []domainLiveRow {
	rows := make([]domainLiveRow, len(targets))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				rows[i] = checkOneDomainLive(client, targets[i], timeout)
			}
		}()
	}
	for i := range targets {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	sort.SliceStable(rows, func(a, b int) bool { return rows[a].Domain < rows[b].Domain })
	return rows
}

func checkOneDomainLive(client domainVerifier, d apiclient.DomainResource, timeout time.Duration) domainLiveRow {
	row := domainLiveRow{Domain: d.Domain, App: d.ServiceName}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	rep, err := client.DomainVerify(ctx, strings.ToLower(d.Domain), nil)
	if err != nil {
		row.DNS, row.Certificate, row.Site = "unknown", "unknown", "unknown"
		row.Next = "check failed: " + err.Error()
		return row
	}
	row.DNS, row.Certificate, row.Site = "-", "-", "-"
	live := true
	for _, l := range rep.Lines {
		v := strings.ToLower(string(l.Verdict))
		switch l.ID {
		case "dns":
			row.DNS = v
		case "certificate":
			row.Certificate = v
		case "response":
			row.Site = v
		default:
			continue
		}
		if l.Verdict != "PASS" {
			live = false
			if row.Next == "" {
				row.Next = l.Next
			}
		}
	}
	row.Live = live && row.DNS == "pass" && row.Certificate == "pass" && row.Site == "pass"
	if row.Live {
		row.Next = "none"
	}
	return row
}

func printLiveRows(out io.Writer, rows []domainLiveRow) {
	if len(rows) == 0 {
		_, _ = fmt.Fprintln(out, "no domains to check")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "DOMAIN\tDNS\tCERTIFICATE\tSITE\tNEXT STEP")
	for _, r := range rows {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.Domain, r.DNS, r.Certificate, r.Site, r.Next)
	}
	_ = tw.Flush()
}
