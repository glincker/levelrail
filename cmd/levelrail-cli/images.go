package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func imagesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s images list [--node N] [--keep N]
  %[1]s images layers <image-id> [--node N]
  %[1]s images gc [--node N] [--keep N] [--min-age-hours H] [--apply]
  %[1]s images verify (<image-id> | --ref repo@sha256:...) [--node N]

Every image on a node with its size, which apps use it and why it is kept.
gc previews by default and only removes images with --apply. Live images,
rollback targets, the newest successful deploys of each app and images used
by a container are never removed.
`, prog)
}

// runImages implements "images": the image inventory and retention.
func runImages(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, imagesUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, imagesUsage(prog))
		return exitOK
	case "list":
		return runImagesList(prog, args[1:], stdout, stderr, lookupEnv)
	case "layers":
		return runImagesLayers(prog, args[1:], stdout, stderr, lookupEnv)
	case "gc":
		return runImagesGC(prog, args[1:], stdout, stderr, lookupEnv)
	case "verify":
		return runImagesVerify(prog, args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown images subcommand %q\n\n%s", prog, args[0], imagesUsage(prog))
	return exitUsage
}

type imagesCall struct {
	client *Client
	node   string
	jsonO  bool
	of     outputFlags
	rest   []string
}

func parseImagesCall(prog, label string, args []string, stderr io.Writer, lookupEnv func(string) (string, bool), define func(*flag.FlagSet)) (imagesCall, int, bool) {
	fs, tokenP, apiP, profileP, jsonP, outP, queryP := apiFlagSet(prog, label, "print the result as JSON to stdout and nothing else", stderr)
	var node string
	fs.StringVar(&node, "node", "", "node ID (default: the control plane's own node)")
	if define != nil {
		define(fs)
	}
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, imagesUsage(prog)) }
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return imagesCall{}, code, false
	}
	return imagesCall{client: apiClientFromFlags(prog, apiURL, token, profile, lookupEnv), node: node, jsonO: jsonOut, of: of, rest: fs.Args()}, exitOK, true
}

func shortImageID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func runImagesList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	keep := -1
	c, code, ok := parseImagesCall(prog, "images list", args, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.IntVar(&keep, "keep", -1, "newest successful deploys per app to keep (default: server setting)")
	})
	if !ok {
		return code
	}
	res, err := c.client.ListImages(bg(), c.node, keep)
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("list images: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, c.of, res, func() {
		_, _ = fmt.Fprintf(stdout, "%d images, %s total, %s reclaimable by retention (keeping the newest %d deploys per app)\n",
			res.Totals.Count, humanBytes(res.Totals.SizeBytes), humanBytes(res.Totals.ReclaimBytes), res.Retention.KeepPerApp)
		tw := tabwriter.NewWriter(stdout, 0, 8, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tTAG\tSIZE\tAPPS\tSTATUS")
		for _, e := range res.Images {
			tag := "<none>"
			if len(e.Tags) > 0 {
				tag = e.Tags[0]
			}
			status := "removable"
			if e.Protected {
				status = e.ProtectReason
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", shortImageID(e.ID), tag, humanBytes(e.SizeBytes), imageAppNames(e.Apps), status)
		}
		_ = tw.Flush()
	})
}

func imageAppNames(uses []apiclient.ImageAppUse) string {
	seen := map[string]bool{}
	var names []string
	for _, u := range uses {
		if !seen[u.App] {
			seen[u.App] = true
			names = append(names, u.App)
		}
	}
	if len(names) == 0 {
		return "-"
	}
	return strings.Join(names, ",")
}

func runImagesLayers(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseImagesCall(prog, "images layers", args, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	if len(c.rest) != 1 {
		return usageError(stderr, prog, "images layers takes one image ID")
	}
	res, err := c.client.GetImageLayers(bg(), c.node, c.rest[0])
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("image layers: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, c.of, res, func() {
		tw := tabwriter.NewWriter(stdout, 0, 8, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "SIZE\tSHARE\tCREATED BY")
		for _, l := range res.Layers {
			share := 0.0
			if res.SizeBytes > 0 {
				share = float64(l.SizeBytes) * 100 / float64(res.SizeBytes)
			}
			_, _ = fmt.Fprintf(tw, "%s\t%.1f%%\t%s\n", humanBytes(l.SizeBytes), share, l.CreatedBy)
		}
		_ = tw.Flush()
		_, _ = fmt.Fprintf(stdout, "total %s\n", humanBytes(res.SizeBytes))
	})
}

func runImagesGC(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	keep, minAge := -1, -1
	var apply bool
	c, code, ok := parseImagesCall(prog, "images gc", args, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.IntVar(&keep, "keep", -1, "newest successful deploys per app to keep (default: server setting)")
		fs.IntVar(&minAge, "min-age-hours", -1, "never remove images younger than this (default: server setting)")
		fs.BoolVar(&apply, "apply", false, "remove the images instead of only previewing")
	})
	if !ok {
		return code
	}
	req := apiclient.ImageGCRequest{Node: c.node, DryRun: !apply}
	if keep >= 0 {
		req.KeepPerApp = &keep
	}
	if minAge >= 0 {
		req.MinAgeHrs = &minAge
	}
	res, err := c.client.RunImageGC(bg(), req)
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("image gc: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, c.of, res, func() {
		if res.DryRun {
			var total int64
			for _, e := range res.Candidates {
				total += e.SizeBytes - e.SharedBytes
				_, _ = fmt.Fprintf(stdout, "would remove %s %s (%s)\n", shortImageID(e.ID), strings.Join(e.Tags, ","), humanBytes(e.SizeBytes))
			}
			_, _ = fmt.Fprintf(stdout, "%d images, %s would be freed. Run with --apply to remove them.\n", len(res.Candidates), humanBytes(total))
			return
		}
		_, _ = fmt.Fprintf(stdout, "removed %d images, freed %s\n", len(res.Removed), humanBytes(res.ReclaimedBytes))
		for _, f := range res.Failed {
			_, _ = fmt.Fprintf(stdout, "could not remove %s: %s\n", shortImageID(f.ID), f.Error)
		}
	})
}

func runImagesVerify(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var ref string
	c, code, ok := parseImagesCall(prog, "images verify", args, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&ref, "ref", "", "verify this registry reference (repo@sha256:...) instead of a local image")
	})
	if !ok {
		return code
	}
	req := apiclient.ImageVerifyRequest{Node: c.node, Ref: ref}
	if ref == "" {
		if len(c.rest) != 1 {
			return usageError(stderr, prog, "images verify takes an image ID or --ref")
		}
		req.ID = c.rest[0]
	}
	res, err := c.client.VerifyImage(bg(), req)
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("verify image: %w", err))
	}
	exit := exitOK
	if res.Status != "verified" && res.Status != "not_configured" {
		exit = exitCheckFailed
	}
	if rc := writeScheduledTaskResult(stdout, stderr, c.of, res, func() {
		_, _ = fmt.Fprintf(stdout, "%s: %s\n", res.Status, res.Message)
	}); rc != exitOK {
		return rc
	}
	return exit
}
