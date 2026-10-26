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

	"golang.org/x/term"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func assetsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s assets <command> [flags]

Assets are files served from an object storage bucket: private by default,
public only when you confirm it.

Commands:
  buckets list [--project P]                  list asset buckets
  buckets create <name> --target ID [flags]   connect a bucket to a storage destination
  buckets show <bucket>                       one bucket with its serving settings
  buckets set <bucket> [flags]                change visibility, serving mode, URLs, referrers or quota
  buckets probe <bucket> [--fix-cors]         check access, upload CORS and public exposure
  buckets delete <bucket> [--yes]             forget the bucket (objects stay at the provider)
  usage <bucket> [--egress-gb N]              stored size and an estimated monthly cost
  ls <bucket> [--prefix P] [--cursor C]       list objects
  upload <bucket> <file> [--key K]            upload a file; large files go straight to the provider
  url <bucket> <key> [--expires D]            origin, CDN or time-limited URL of an object
  rm <bucket> <key>                           delete an object

A <bucket> is the bucket name or its id.
Run '%[1]s assets <command> --help' for the flags of a command.
`, prog)
}

func runAssets(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprint(stderr, assetsUsage(prog))
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	rest := args[1:]
	switch args[0] {
	case "buckets":
		return runAssetsBuckets(prog, rest, stdout, stderr, lookupEnv)
	case "usage":
		return assetsUsageCmd(prog, rest, stdout, stderr, lookupEnv)
	case "ls":
		return assetsLs(prog, rest, stdout, stderr, lookupEnv)
	case "upload":
		return assetsUpload(prog, rest, stdout, stderr, lookupEnv)
	case "url":
		return assetsURL(prog, rest, stdout, stderr, lookupEnv)
	case "rm":
		return assetsRm(prog, rest, stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s assets: unknown command %q\n\n%s", prog, args[0], assetsUsage(prog))
	return exitUsage
}

func runAssetsBuckets(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprint(stderr, assetsUsage(prog))
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	rest := args[1:]
	switch args[0] {
	case "list":
		return assetsBucketsList(prog, rest, stdout, stderr, lookupEnv)
	case "create":
		return assetsBucketsCreate(prog, rest, stdout, stderr, lookupEnv)
	case "show":
		return assetsBucketsShow(prog, rest, stdout, stderr, lookupEnv)
	case "set":
		return assetsBucketsSet(prog, rest, stdout, stderr, lookupEnv)
	case "probe":
		return assetsBucketsProbe(prog, rest, stdout, stderr, lookupEnv)
	case "delete":
		return assetsBucketsDelete(prog, rest, stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s assets buckets: unknown command %q\n\n%s", prog, args[0], assetsUsage(prog))
	return exitUsage
}

func assetsCmdUsage(prog, sub, synopsis, flags string) string {
	return fmt.Sprintf("Usage:\n  %s assets %s %s\n%s%s", prog, sub, synopsis, flags, commonFlagsHelp)
}

// assetsCall parses flags and checks the positional count; ok=false means return code as is.
func assetsCall(prog, sub, synopsis, flags string, want int, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), bind func(*flag.FlagSet)) (cliCall, []string, int, bool) {
	c, code, ok := parseCLICall(prog, "assets "+sub, assetsCmdUsage(prog, sub, synopsis, flags), args, stdout, stderr, lookupEnv, bind)
	if !ok {
		return cliCall{}, nil, code, false
	}
	pos := c.fs.Args()
	if len(pos) != want {
		return cliCall{}, nil, c.invalid("usage: %s assets %s %s", prog, sub, synopsis), false
	}
	return c, pos, exitOK, true
}

// resolveAssetBucket accepts a bucket name or id; names are unique per control plane.
func resolveAssetBucket(ctx context.Context, client *apiclient.Client, ref string) (apiclient.AssetBucket, error) {
	list, _, err := client.ListAssetBuckets(ctx, "")
	if err != nil {
		return apiclient.AssetBucket{}, err
	}
	for _, b := range list {
		if b.ID == ref || b.Name == ref {
			return b, nil
		}
	}
	return apiclient.AssetBucket{}, newValidationError("asset bucket %q not found: run 'assets buckets list' to see the names", ref)
}

func assetsBucketsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var project *string
	c, _, code, ok := assetsCall(prog, "buckets list", "[--project P]", "\nFlags:\n  --project P   only buckets of this project\n", 0, args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		project = fs.String("project", "", "only buckets of this project")
	})
	if !ok {
		return code
	}
	list, raw, err := c.client.ListAssetBuckets(context.Background(), *project)
	if err != nil {
		return c.fail(err)
	}
	return c.render(raw, func() { printAssetBuckets(c.stdout, list) })
}

type assetBucketFlags struct {
	serving, publicBase, cdnBase, project *string
	referrers                             *string
	allowNoReferer                        *bool
	quota                                 *int64
	confirmName                           *string
}

func bindAssetBucketFlags(fs *flag.FlagSet) assetBucketFlags {
	return assetBucketFlags{
		serving:        fs.String("serving", "", "serving mode: direct or cdn"),
		publicBase:     fs.String("public-base-url", "", "public base URL of the bucket at the provider"),
		cdnBase:        fs.String("cdn-base-url", "", "CDN base URL (needed for --serving cdn)"),
		project:        fs.String("project", "", "project the bucket belongs to"),
		referrers:      fs.String("referrer-allow", "", "comma separated hosts allowed to embed public assets"),
		allowNoReferer: fs.Bool("allow-no-referer", false, "accept requests without a Referer when referrers are restricted"),
		quota:          fs.Int64("quota-bytes", 0, "storage quota in bytes (0 means none)"),
		confirmName:    fs.String("confirm-name", "", "the bucket name, required to make a bucket public"),
	}
}

const assetBucketFlagsHelp = `
Flags:
  --serving direct|cdn      serving mode
  --public-base-url URL     public base URL of the bucket at the provider
  --cdn-base-url URL        CDN base URL (needed for --serving cdn)
  --project P               project the bucket belongs to
  --referrer-allow H,H      hosts allowed to embed public assets
  --allow-no-referer        accept requests without a Referer when referrers are restricted
  --quota-bytes N           storage quota in bytes
  --confirm-name NAME       the bucket name, required to make a bucket public
`

func assetsBucketsCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var f assetBucketFlags
	var target, prefix *string
	var public *bool
	c, pos, code, ok := assetsCall(prog, "buckets create", "<name> --target ID [--prefix P] [--public] [flags]", "\n  --target ID   storage destination id (required)\n  --prefix P    key prefix inside the provider bucket\n  --public      make the bucket public (asks you to type the name)\n"+assetBucketFlagsHelp, 1, args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		target = fs.String("target", "", "storage destination id")
		prefix = fs.String("prefix", "", "key prefix inside the provider bucket")
		public = fs.Bool("public", false, "make the bucket public")
		f = bindAssetBucketFlags(fs)
	})
	if !ok {
		return code
	}
	if *target == "" {
		return c.invalid("--target is required")
	}
	req := apiclient.AssetBucketCreate{
		Name: pos[0], TargetID: *target, Prefix: *prefix, ProjectID: *f.project, ServingMode: *f.serving,
		PublicBaseURL: *f.publicBase, CDNBaseURL: *f.cdnBase, ReferrerAllow: splitCSV(*f.referrers), QuotaBytes: *f.quota,
	}
	if flagWasSet(c.fs, "allow-no-referer") {
		req.AllowNoReferer = f.allowNoReferer
	}
	if *public {
		name, err := assetsConfirmName(c.stderr, pos[0], *f.confirmName)
		if err != nil {
			return c.fail(err)
		}
		req.Visibility, req.ConfirmName = "public", name
	}
	b, raw, err := c.client.CreateAssetBucket(context.Background(), req)
	if err != nil {
		return c.fail(err)
	}
	return c.render(raw, func() {
		_, _ = fmt.Fprintf(c.stdout, "created asset bucket %s (%s, %s)\n", b.Name, b.Visibility, b.ServingMode)
	})
}

func assetsBucketsShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, pos, code, ok := assetsCall(prog, "buckets show", "<bucket>", "", 1, args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	ctx := context.Background()
	b, err := resolveAssetBucket(ctx, c.client, pos[0])
	if err != nil {
		return c.fail(err)
	}
	b, raw, err := c.client.GetAssetBucket(ctx, b.ID)
	if err != nil {
		return c.fail(err)
	}
	return c.render(raw, func() { printAssetBucket(c.stdout, b) })
}

func assetsBucketsSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var f assetBucketFlags
	var public, private *bool
	c, pos, code, ok := assetsCall(prog, "buckets set", "<bucket> [--public|--private] [flags]", "\n  --public      make the bucket public (asks you to type the name)\n  --private     make the bucket private\n"+assetBucketFlagsHelp, 1, args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		public = fs.Bool("public", false, "make the bucket public")
		private = fs.Bool("private", false, "make the bucket private")
		f = bindAssetBucketFlags(fs)
	})
	if !ok {
		return code
	}
	if *public && *private {
		return c.invalid("--public and --private cannot be used together")
	}
	ctx := context.Background()
	b, err := resolveAssetBucket(ctx, c.client, pos[0])
	if err != nil {
		return c.fail(err)
	}
	var req apiclient.AssetBucketUpdate
	changed := false
	setStr := func(flagName string, dst **string, v *string) {
		if flagWasSet(c.fs, flagName) {
			*dst, changed = v, true
		}
	}
	setStr("serving", &req.ServingMode, f.serving)
	setStr("public-base-url", &req.PublicBaseURL, f.publicBase)
	setStr("cdn-base-url", &req.CDNBaseURL, f.cdnBase)
	setStr("project", &req.ProjectID, f.project)
	if flagWasSet(c.fs, "referrer-allow") {
		list := splitCSV(*f.referrers)
		if list == nil {
			list = []string{}
		}
		req.ReferrerAllow, changed = &list, true
	}
	if flagWasSet(c.fs, "allow-no-referer") {
		req.AllowNoReferer, changed = f.allowNoReferer, true
	}
	if flagWasSet(c.fs, "quota-bytes") {
		req.QuotaBytes, changed = f.quota, true
	}
	if *private {
		v := "private"
		req.Visibility, changed = &v, true
	}
	if *public {
		v := "public"
		req.Visibility, changed = &v, true
		if b.Visibility != "public" {
			name, err := assetsConfirmName(c.stderr, b.Name, *f.confirmName)
			if err != nil {
				return c.fail(err)
			}
			req.ConfirmName = name
		}
	}
	if !changed {
		return c.invalid("nothing to change: pass at least one flag")
	}
	updated, raw, err := c.client.UpdateAssetBucket(ctx, b.ID, req)
	if err != nil {
		return c.fail(err)
	}
	return c.render(raw, func() { printAssetBucket(c.stdout, updated) })
}

// assetsConfirmName never confirms for the caller: it takes --confirm-name or asks on a terminal.
func assetsConfirmName(stderr io.Writer, bucketName, given string) (string, error) {
	if given != "" {
		return given, nil
	}
	if f, ok := stdinSource.(*os.File); !ok || !term.IsTerminal(int(f.Fd())) { //nolint:gosec // a terminal file descriptor always fits an int
		return "", newValidationError("making a bucket public needs --confirm-name %s", bucketName)
	}
	_, _ = fmt.Fprintf(stderr, "Anyone with the URL can read a public bucket. Type the bucket name (%s) to confirm: ", bucketName)
	line, _ := bufio.NewReader(stdinSource).ReadString('\n')
	typed := strings.TrimSpace(line)
	if typed != bucketName {
		return "", errors.New("cancelled: the name did not match")
	}
	return typed, nil
}

func assetsBucketsProbe(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var origin *string
	var fixCORS *bool
	c, pos, code, ok := assetsCall(prog, "buckets probe", "<bucket> [--origin URL] [--fix-cors]", "\n  --origin URL   page origin to check browser uploads from (default: the dashboard URL)\n  --fix-cors     apply the upload CORS rule at the provider when it is missing\n", 1, args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		origin = fs.String("origin", "", "page origin to check browser uploads from")
		fixCORS = fs.Bool("fix-cors", false, "apply the upload CORS rule at the provider when it is missing")
	})
	if !ok {
		return code
	}
	ctx := context.Background()
	b, err := resolveAssetBucket(ctx, c.client, pos[0])
	if err != nil {
		return c.fail(err)
	}
	res, raw, err := c.client.ProbeAssetBucket(ctx, b.ID, *origin, *fixCORS)
	if err != nil {
		return c.fail(err)
	}
	rc := c.render(raw, func() { printAssetProbe(c.stdout, res) })
	if rc == exitOK && !res.OK {
		return exitCheckFailed
	}
	return rc
}

func assetsBucketsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var yes *bool
	c, pos, code, ok := assetsCall(prog, "buckets delete", "<bucket> [--yes]", "\n  --yes   skip the confirmation\n\nOnly the connection is removed; objects stay at the provider.\n", 1, args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		yes = fs.Bool("yes", false, "skip the confirmation")
	})
	if !ok {
		return code
	}
	ctx := context.Background()
	b, err := resolveAssetBucket(ctx, c.client, pos[0])
	if err != nil {
		return c.fail(err)
	}
	if !*yes {
		if err := stackConfirm(c.stderr, fmt.Sprintf("Forget asset bucket %s (objects stay at the provider)", b.Name)); err != nil {
			return c.fail(err)
		}
	}
	if err := c.client.DeleteAssetBucket(ctx, b.ID); err != nil {
		return c.fail(err)
	}
	return c.render(map[string]string{"deleted": b.Name}, func() { _, _ = fmt.Fprintf(c.stdout, "deleted asset bucket %s\n", b.Name) })
}
