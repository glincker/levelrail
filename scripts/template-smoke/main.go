// Command template-smoke starts catalog templates under real Docker and
// reports per template whether services stay running, exit, pass their
// healthcheck or fail to pull. Dev and CI only: the control plane never
// drives the docker CLI.
//
//	go run ./scripts/template-smoke -only ntfy,gitea -pull -md report.md
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

type options struct {
	only       map[string]bool
	category   string
	settle     time.Duration
	timeout    time.Duration
	parallel   int
	pull       bool
	maxImageGB float64
	shard      int
	shards     int
	jsonOut    string
	mdOut      string
	project    string
	plan       bool
	subnetBase string
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	opts, err := parseFlags(args, stderr)
	if err != nil {
		return 2
	}
	selected := selectTemplates(catalog.Templates, opts)
	if len(selected) == 0 {
		_, _ = fmt.Fprintln(stderr, "template-smoke: no templates selected")
		return 2
	}
	if opts.plan {
		printPlan(ctx, selected, stdout)
		return 0
	}
	_, _ = fmt.Fprintf(stderr, "template-smoke: %d templates, %d at a time\n", len(selected), opts.parallel)

	results := make([]result, len(selected))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < opts.parallel; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				r := smokeTemplate(ctx, selected[i], opts)
				_, _ = fmt.Fprintf(stderr, "  %-28s %s %s\n", r.ID, r.Status, r.Summary)
				results[i] = r
			}
		}()
	}
	for i := range selected {
		if ctx.Err() != nil {
			break
		}
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	if err := writeReports(results, opts, stdout); err != nil {
		_, _ = fmt.Fprintln(stderr, "template-smoke:", err)
		return 2
	}
	for _, r := range results {
		if r.Status == statusFail {
			return 1
		}
	}
	return 0
}

func parseFlags(args []string, stderr io.Writer) (options, error) {
	fs := flag.NewFlagSet("template-smoke", flag.ContinueOnError)
	fs.SetOutput(stderr)
	only := fs.String("only", "", "comma-separated template ids to run (default: all)")
	category := fs.String("category", "", "only templates in this category (case-insensitive)")
	settle := fs.Duration("settle", 20*time.Second, "how long every service must stay up after start")
	timeout := fs.Duration("timeout", 4*time.Minute, "per-template budget: pull, start, settle and teardown")
	parallel := fs.Int("parallel", 2, "templates run at once")
	pull := fs.Bool("pull", false, "pull images that are not present locally (otherwise such templates are skipped)")
	maxGB := fs.Float64("max-image-gb", 2, "skip a template with an image larger than this (local size, or compressed registry size)")
	shard := fs.String("shard", "", "run one shard of the selection, as index/total (1-based), for CI matrices")
	jsonOut := fs.String("json", "", "write the JSON report to this file")
	mdOut := fs.String("md", "", "write the markdown report to this file")
	project := fs.String("project-prefix", "lrsmoke", "compose project name prefix; every resource created carries it")
	subnetBase := fs.String("subnet-base", "10.231", "first two octets of the /24 each run's default network gets")
	plan := fs.Bool("plan", false, "list the selected templates and whether their images are local, without running anything")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	o := options{
		category: *category, settle: *settle, timeout: *timeout, parallel: max(1, *parallel),
		pull: *pull, maxImageGB: *maxGB, jsonOut: *jsonOut, mdOut: *mdOut, project: *project, plan: *plan, subnetBase: *subnetBase,
		shard: 1, shards: 1,
	}
	if *only != "" {
		o.only = map[string]bool{}
		for _, id := range strings.Split(*only, ",") {
			if id = strings.TrimSpace(id); id != "" {
				o.only[id] = true
			}
		}
	}
	if *shard != "" {
		i, n, err := parseShard(*shard)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "template-smoke:", err)
			return options{}, err
		}
		o.shard, o.shards = i, n
	}
	return o, nil
}

func parseShard(s string) (int, int, error) {
	a, b, ok := strings.Cut(s, "/")
	i, err1 := strconv.Atoi(a)
	n, err2 := strconv.Atoi(b)
	if !ok || err1 != nil || err2 != nil || n < 1 || i < 1 || i > n {
		return 0, 0, errors.New("-shard must be index/total, for example 2/4")
	}
	return i, n, nil
}

// selectTemplates filters, sorts by id and keeps one shard of the result.
func selectTemplates(all []catalog.Template, o options) []catalog.Template {
	var out []catalog.Template
	for _, t := range all {
		if o.only != nil && !o.only[t.ID] {
			continue
		}
		if o.category != "" && !strings.EqualFold(t.Category, o.category) {
			continue
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if o.shards <= 1 {
		return out
	}
	var shard []catalog.Template
	for i, t := range out {
		if i%o.shards == o.shard-1 {
			shard = append(shard, t)
		}
	}
	return shard
}
