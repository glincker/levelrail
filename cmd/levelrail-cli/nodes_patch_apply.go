package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runNodesPatch dispatches "nodes patch preview|apply|runs <node>".
func runNodesPatch(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, nodesPatchUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "preview", "apply", "runs":
		return runNodesPatchVerb(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, nodesPatchUsage(prog))
		return exitOK
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown nodes patch command %q\n\n%s", prog, args[0], nodesPatchUsage(prog))
	return exitUsage
}

func runNodesPatchVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "nodes patch " + verb
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesPatchUsage(prog)) }
	includeAll := fs.Bool("include-all", false, "include non-security updates and kernel, docker and containerd packages")
	refresh := fs.Bool("refresh", false, "refresh package lists before the preview")
	yes := fs.Bool("yes", false, "apply the previewed packages; without it apply only shows the preview")
	reboot := fs.Bool("reboot-if-required", false, "reboot after the upgrade when needed, only if no apps or databases run on the node")
	previewID := fs.String("preview", "", "apply this earlier preview id instead of taking a fresh one")
	noWait := fs.Bool("no-wait", false, "return once the run starts instead of waiting for it to finish")

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, label, "node id")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	if verb == "runs" {
		runs, err := client.ListNodePatchRuns(ctx, id)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("list patch runs for node %q: %w", id, err))
		}
		return renderPatchResult(stdout, stderr, of, runs, func() {
			for _, r := range runs {
				_, _ = fmt.Fprintln(stdout, formatPatchRunLine(r))
			}
		})
	}

	var preview apiclient.NodePatchPreview
	if verb == "preview" || *previewID == "" {
		p, err := client.PreviewNodePatch(ctx, id, *includeAll, *refresh)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("preview patches for node %q: %w", id, err))
		}
		preview = p
	}
	if verb == "preview" || !*yes {
		if verb == "apply" && !jsonOut {
			_, _ = fmt.Fprintln(stderr, "not applied: repeat with --yes to install the packages below")
		}
		return renderPatchResult(stdout, stderr, of, preview, func() { _, _ = fmt.Fprint(stdout, formatPatchPreview(preview)) })
	}
	pid := *previewID
	if pid == "" {
		pid = preview.PreviewID
	}
	run, err := client.ApplyNodePatch(ctx, id, apiclient.NodePatchApplyRequest{PreviewID: pid, IncludeAll: *includeAll, RebootIfRequired: *reboot, Confirm: true})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("apply patches on node %q: %w", id, err))
	}
	if !*noWait {
		run = waitPatchRun(func() ([]apiclient.NodePatchRun, error) { return client.ListNodePatchRuns(ctx, id) }, run, func() { time.Sleep(patchWaitPoll) })
	}
	code := renderPatchResult(stdout, stderr, of, run, func() { _, _ = fmt.Fprint(stdout, formatPatchRun(run)) })
	if code != exitOK {
		return code
	}
	switch run.Status {
	case patchStatusUnknown:
		_, _ = fmt.Fprintf(stderr, "%s: lost track of patch run %s (the control plane may have restarted); check: %s nodes patch runs %s and %s nodes patch-status %s\n", prog, run.ID, prog, id, prog, id)
		return exitCheckFailed
	case "failed", "timed_out":
		return exitCheckFailed
	}
	return exitOK
}

const (
	patchStatusUnknown = "unknown"
	patchWaitPoll      = 2 * time.Second // CLI-side poll pace only; the server bounds the run itself
)

// waitPatchRun polls run history; a list error or a vanished run means the outcome is unknown, never success.
func waitPatchRun(list func() ([]apiclient.NodePatchRun, error), run apiclient.NodePatchRun, sleep func()) apiclient.NodePatchRun {
	for run.Status == "running" {
		sleep()
		runs, err := list()
		if err != nil {
			run.Status = patchStatusUnknown
			return run
		}
		found := false
		for _, r := range runs {
			if r.ID == run.ID {
				run, found = r, true
			}
		}
		if !found {
			run.Status = patchStatusUnknown
		}
	}
	return run
}

func renderPatchResult(stdout, stderr io.Writer, of outputFlags, data any, human func()) int {
	if err := renderResult(stdout, of.Format, of.Query, data, human); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func patchFlags(p apiclient.NodePatchPackage) string {
	var f []string
	if p.Security {
		f = append(f, "security")
	}
	if p.Kernel {
		f = append(f, "kernel, reboot")
	}
	if p.ContainerRuntime {
		f = append(f, "container runtime")
	}
	return strings.Join(f, ", ")
}

func formatPatchPreview(p apiclient.NodePatchPreview) string {
	var b strings.Builder
	scope := "security updates"
	if p.IncludeAll {
		scope = "all updates"
	}
	fmt.Fprintf(&b, "%s on %s (%s): %d to install\n", scope, p.NodeID, p.Manager, len(p.Packages))
	for _, pkg := range p.Packages {
		fmt.Fprintf(&b, "  %s %s -> %s  [%s]\n", pkg.Name, pkg.CurrentVersion, pkg.NewVersion, patchFlags(pkg))
	}
	if len(p.Held) > 0 {
		fmt.Fprintf(&b, "held (needs --include-all):\n")
		for _, pkg := range p.Held {
			fmt.Fprintf(&b, "  %s -> %s  [%s]\n", pkg.Name, pkg.NewVersion, patchFlags(pkg))
		}
	}
	if p.OtherAvailable > 0 {
		fmt.Fprintf(&b, "%d non-security updates not included (use --include-all)\n", p.OtherAvailable)
	}
	if p.RebootRequiredNow || p.RebootLikely {
		fmt.Fprintf(&b, "a reboot is or will be required\n")
	}
	fmt.Fprintf(&b, "preview %s valid until %s\n", p.PreviewID, p.ExpiresAt.Local().Format(time.RFC3339))
	return b.String()
}

func formatPatchRunLine(r apiclient.NodePatchRun) string {
	return fmt.Sprintf("%s  %s  %s  %d packages  by %s", r.ID, r.StartedAt.Local().Format(time.RFC3339), r.Status, len(r.Packages), r.Actor)
}

func formatPatchRun(r apiclient.NodePatchRun) string {
	var b strings.Builder
	fmt.Fprintln(&b, formatPatchRunLine(r))
	if r.Error != "" {
		fmt.Fprintf(&b, "error: %s\n", r.Error)
	}
	if r.OutputTail != "" {
		fmt.Fprintf(&b, "output (tail):\n%s\n", r.OutputTail)
	}
	if r.RebootNote != "" {
		fmt.Fprintf(&b, "reboot: %s\n", r.RebootNote)
	}
	if s := r.PatchStatusAfter; s != nil {
		fmt.Fprintf(&b, "now pending: %d updates, %d security\n", s.Total, s.Security)
	}
	return b.String()
}

func nodesPatchUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes patch preview <id> [--include-all] [--refresh] [flags]
  %[1]s nodes patch apply <id> --yes [--include-all] [--reboot-if-required] [--preview ID] [--no-wait] [flags]
  %[1]s nodes patch runs <id> [flags]

Preview lists exactly which OS packages would change (security updates only by
default; kernel, docker and containerd upgrades are held unless --include-all).
Apply installs that exact list and refuses when the list changed since the
preview, a deploy is running, or another patch run is active. It never reboots
unless --reboot-if-required is set and no apps or databases run on the node.
Today only the control plane's own host can be patched.

Flags:
  --include-all            include non-security, kernel and container runtime updates
  --refresh                refresh package lists before the preview
  --yes                    apply; without it apply only prints the preview
  --reboot-if-required     reboot when the upgrade needs it and the node is drained
  --preview string         apply an earlier preview id
  --no-wait                do not wait for the run to finish
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string         control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string         named credentials profile to read
  --json                   print the result as JSON
  --output string          output format: json, table, or text
  --query string           JMESPath expression to filter the result
  -h, --help               show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}
