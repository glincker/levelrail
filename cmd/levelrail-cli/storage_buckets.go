package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func storageBucketsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s storage buckets list [--app NAME]                  list buckets
  %[1]s storage buckets create --name N --destination ID [--app NAME] [--env-prefix P] [--public] [--quota-mb N] [--expire-days N]
  %[1]s storage buckets update <id> [--public=true|false] [--quota-mb N] [--expire-days N]
  %[1]s storage buckets delete <id> [--purge]              remove a bucket, --purge also deletes its files
  %[1]s storage buckets bind <id> --app NAME [--env-prefix P]   inject its S3 env vars into an app at next container start
  %[1]s storage buckets unbind <id> --app NAME
  %[1]s storage buckets usage <id>                         stored size against the quota
  %[1]s storage buckets ls <id> [--prefix P]               list files
  %[1]s storage buckets put <id> <key> <file>              upload a file
  %[1]s storage buckets get <id> <key> [--out FILE]        download a file
  %[1]s storage buckets rm <id> <key>                      delete a file
  %[1]s storage buckets presign <id> <key> [--method GET|PUT] [--expires 15m]
  %[1]s storage buckets sweep <id>                         run the expire-after-N-days rule now
%[2]s`, prog, commonFlagsHelp)
}

const bytesPerMB int64 = 1 << 20

func runStorageBuckets(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, storageBucketsUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, storageBucketsUsage(prog))
		return exitOK
	case "list":
		return runBucketsList(prog, rest, stdout, stderr, lookupEnv)
	case "create":
		return runBucketsCreate(prog, rest, stdout, stderr, lookupEnv)
	case "update":
		return runBucketsUpdate(prog, rest, stdout, stderr, lookupEnv)
	case "delete":
		return runBucketsDelete(prog, rest, stdout, stderr, lookupEnv)
	case "bind", "unbind":
		return runBucketsBind(prog, args[0], rest, stdout, stderr, lookupEnv)
	case "usage":
		return runBucketsUsage(prog, rest, stdout, stderr, lookupEnv)
	case "ls":
		return runBucketsLs(prog, rest, stdout, stderr, lookupEnv)
	case "put":
		return runBucketsPut(prog, rest, stdout, stderr, lookupEnv)
	case "get":
		return runBucketsGet(prog, rest, stdout, stderr, lookupEnv)
	case "rm":
		return runBucketsRm(prog, rest, stdout, stderr, lookupEnv)
	case "presign":
		return runBucketsPresign(prog, rest, stdout, stderr, lookupEnv)
	case "sweep":
		return runBucketsSweep(prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown storage buckets subcommand %q\n\n%s", prog, args[0], storageBucketsUsage(prog))
		return exitUsage
	}
}

func runBucketsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var app string
	c, code, ok := parseCLICall(prog, "storage buckets list", storageBucketsUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&app, "app", "", "only buckets bound to this app")
	})
	if !ok {
		return code
	}
	buckets, err := c.client.ListStorageBuckets(context.Background(), app)
	if err != nil {
		return c.fail(fmt.Errorf("list buckets: %w", err))
	}
	return c.render(buckets, func() {
		if len(buckets) == 0 {
			_, _ = fmt.Fprintln(stdout, "no buckets")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tNAME\tACCESS\tQUOTA\tEXPIRE\tAPPS")
		for _, b := range buckets {
			access, quota, expire := "private", "none", "never"
			if b.Public {
				access = "public"
			}
			if b.QuotaBytes > 0 {
				quota = fmt.Sprintf("%d MB", b.QuotaBytes/bytesPerMB)
			}
			if b.LifecycleDays > 0 {
				expire = fmt.Sprintf("%dd", b.LifecycleDays)
			}
			var apps []string
			for _, a := range b.Apps {
				apps = append(apps, a.AppName)
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", b.ID, b.Name, access, quota, expire, strings.Join(apps, ","))
		}
		_ = tw.Flush()
	})
}

func runBucketsCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var name, dest, app, envPrefix string
	var public bool
	var quotaMB int64
	var days int
	c, code, ok := parseCLICall(prog, "storage buckets create", storageBucketsUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&name, "name", "", "bucket name (required)")
		fs.StringVar(&dest, "destination", "", "storage destination id (required, see storage list)")
		fs.StringVar(&app, "app", "", "bind to this app right away")
		fs.StringVar(&envPrefix, "env-prefix", "", "env var prefix for the binding (default S3)")
		fs.BoolVar(&public, "public", false, "serve files without authentication")
		fs.Int64Var(&quotaMB, "quota-mb", 0, "size limit in MB, 0 for none")
		fs.IntVar(&days, "expire-days", 0, "delete files older than this many days, 0 for never")
	})
	if !ok {
		return code
	}
	if name == "" || dest == "" {
		return c.invalid("--name and --destination are required")
	}
	b, err := c.client.CreateStorageBucket(context.Background(), apiclient.CreateStorageBucketRequest{
		Name: name, TargetID: dest, AppName: app, EnvPrefix: envPrefix, Public: public, QuotaBytes: quotaMB * bytesPerMB, LifecycleDays: days,
	})
	if err != nil {
		return c.fail(fmt.Errorf("create bucket %q: %w", name, err))
	}
	return c.render(b, func() {
		_, _ = fmt.Fprintf(stdout, "bucket %q created (id %s)\n", b.Name, b.ID)
		if len(b.EnvKeys) > 0 {
			_, _ = fmt.Fprintf(stdout, "bound to %s; env vars %s arrive at the next container start\n", app, strings.Join(b.EnvKeys, ", "))
		}
	})
}

func runBucketsUpdate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var public bool
	var quotaMB int64
	var days int
	c, code, ok := parseCLICall(prog, "storage buckets update", storageBucketsUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.BoolVar(&public, "public", false, "serve files without authentication")
		fs.Int64Var(&quotaMB, "quota-mb", 0, "size limit in MB, 0 for none")
		fs.IntVar(&days, "expire-days", 0, "delete files older than this many days, 0 for never")
	})
	if !ok {
		return code
	}
	id, ok := requireOneArg(c.fs, stderr, prog, "storage buckets update", "bucket id")
	if !ok {
		return exitUsage
	}
	req := apiclient.UpdateStorageBucketRequest{}
	c.fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "public":
			req.Public = &public
		case "quota-mb":
			q := quotaMB * bytesPerMB
			req.QuotaBytes = &q
		case "expire-days":
			req.LifecycleDays = &days
		}
	})
	if req.Public == nil && req.QuotaBytes == nil && req.LifecycleDays == nil {
		return c.invalid("pass at least one of --public, --quota-mb, --expire-days")
	}
	b, err := c.client.UpdateStorageBucket(context.Background(), id, req)
	if err != nil {
		return c.fail(fmt.Errorf("update bucket %q: %w", id, err))
	}
	return c.render(b, func() { _, _ = fmt.Fprintf(stdout, "bucket %q updated\n", b.Name) })
}

func runBucketsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var purge bool
	c, code, ok := parseCLICall(prog, "storage buckets delete", storageBucketsUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.BoolVar(&purge, "purge", false, "also delete every file in the bucket")
	})
	if !ok {
		return code
	}
	id, ok := requireOneArg(c.fs, stderr, prog, "storage buckets delete", "bucket id")
	if !ok {
		return exitUsage
	}
	if err := c.client.DeleteStorageBucket(context.Background(), id, purge); err != nil {
		return c.fail(fmt.Errorf("delete bucket %q: %w", id, err))
	}
	return c.render(map[string]bool{"deleted": true}, func() { _, _ = fmt.Fprintf(stdout, "bucket %q deleted\n", id) })
}

func runBucketsBind(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var app, envPrefix string
	label := "storage buckets " + verb
	c, code, ok := parseCLICall(prog, label, storageBucketsUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&app, "app", "", "app name (required)")
		fs.StringVar(&envPrefix, "env-prefix", "", "env var prefix (default S3)")
	})
	if !ok {
		return code
	}
	id, ok := requireOneArg(c.fs, stderr, prog, label, "bucket id")
	if !ok {
		return exitUsage
	}
	if app == "" {
		return c.invalid("--app is required")
	}
	if verb == "unbind" {
		if err := c.client.UnbindAppBucket(context.Background(), app, id); err != nil {
			return c.fail(fmt.Errorf("unbind bucket %q from %q: %w", id, app, err))
		}
		return c.render(map[string]bool{"unbound": true}, func() { _, _ = fmt.Fprintf(stdout, "bucket %q unbound from %s\n", id, app) })
	}
	b, err := c.client.BindAppBucket(context.Background(), app, id, envPrefix)
	if err != nil {
		return c.fail(fmt.Errorf("bind bucket %q to %q: %w", id, app, err))
	}
	return c.render(b, func() {
		_, _ = fmt.Fprintf(stdout, "bucket %q bound to %s; env vars %s arrive at the next container start\n", b.Name, app, strings.Join(b.EnvKeys, ", "))
	})
}

func runBucketsUsage(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "storage buckets usage", storageBucketsUsage(prog), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	id, ok := requireOneArg(c.fs, stderr, prog, "storage buckets usage", "bucket id")
	if !ok {
		return exitUsage
	}
	u, err := c.client.BucketUsage(context.Background(), id)
	if err != nil {
		return c.fail(fmt.Errorf("bucket usage %q: %w", id, err))
	}
	return c.render(u, func() {
		quota := "no quota"
		if u.QuotaBytes > 0 {
			quota = fmt.Sprintf("of %d MB", u.QuotaBytes/bytesPerMB)
		}
		_, _ = fmt.Fprintf(stdout, "%d files, %d bytes %s\n", u.Objects, u.Bytes, quota)
	})
}

func runBucketsLs(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var prefix string
	c, code, ok := parseCLICall(prog, "storage buckets ls", storageBucketsUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&prefix, "prefix", "", "only keys starting with this")
	})
	if !ok {
		return code
	}
	id, ok := requireOneArg(c.fs, stderr, prog, "storage buckets ls", "bucket id")
	if !ok {
		return exitUsage
	}
	page, err := c.client.ListBucketObjects(context.Background(), id, prefix, "")
	if err != nil {
		return c.fail(fmt.Errorf("list files in %q: %w", id, err))
	}
	return c.render(page, func() {
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "KEY\tSIZE\tMODIFIED")
		for _, o := range page.Objects {
			_, _ = fmt.Fprintf(tw, "%s\t%d\t%s\n", o.Key, o.Size, o.LastModified.Format("2006-01-02 15:04"))
		}
		_ = tw.Flush()
		if page.Next != "" {
			_, _ = fmt.Fprintln(stdout, "more files exist, narrow with --prefix")
		}
	})
}

func runBucketsPut(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "storage buckets put", storageBucketsUsage(prog), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	rest, ok := requireArgs(c.fs, stderr, prog, "storage buckets put", "<id> <key> <file>", 3)
	if !ok {
		return exitUsage
	}
	data, err := os.ReadFile(rest[2]) //nolint:gosec // the operator names the file to upload
	if err != nil {
		return c.fail(fmt.Errorf("read %s: %w", rest[2], err))
	}
	o, err := c.client.UploadBucketObject(context.Background(), rest[0], rest[1], data)
	if err != nil {
		return c.fail(fmt.Errorf("upload %q: %w", rest[1], err))
	}
	return c.render(o, func() { _, _ = fmt.Fprintf(stdout, "uploaded %s (%d bytes)\n", o.Key, o.Size) })
}

func runBucketsGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var out string
	c, code, ok := parseCLICall(prog, "storage buckets get", storageBucketsUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&out, "out", "", "write to this file instead of stdout")
	})
	if !ok {
		return code
	}
	rest, ok := requireArgs(c.fs, stderr, prog, "storage buckets get", "<id> <key>", 2)
	if !ok {
		return exitUsage
	}
	data, err := c.client.DownloadBucketObject(context.Background(), rest[0], rest[1])
	if err != nil {
		return c.fail(fmt.Errorf("download %q: %w", rest[1], err))
	}
	if out == "" {
		_, _ = stdout.Write(data)
		return exitOK
	}
	if err := os.WriteFile(out, data, 0o600); err != nil {
		return c.fail(fmt.Errorf("write %s: %w", out, err))
	}
	_, _ = fmt.Fprintf(stderr, "saved %d bytes to %s\n", len(data), out)
	return exitOK
}

func runBucketsRm(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "storage buckets rm", storageBucketsUsage(prog), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	rest, ok := requireArgs(c.fs, stderr, prog, "storage buckets rm", "<id> <key>", 2)
	if !ok {
		return exitUsage
	}
	if err := c.client.DeleteBucketObject(context.Background(), rest[0], rest[1]); err != nil {
		return c.fail(fmt.Errorf("delete %q: %w", rest[1], err))
	}
	return c.render(map[string]bool{"deleted": true}, func() { _, _ = fmt.Fprintf(stdout, "deleted %s\n", rest[1]) })
}

func runBucketsPresign(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var method, expires string
	c, code, ok := parseCLICall(prog, "storage buckets presign", storageBucketsUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&method, "method", "GET", "GET to download or PUT to upload")
		fs.StringVar(&expires, "expires", "15m", "how long the URL works, up to 168h")
	})
	if !ok {
		return code
	}
	rest, ok := requireArgs(c.fs, stderr, prog, "storage buckets presign", "<id> <key>", 2)
	if !ok {
		return exitUsage
	}
	ttl, err := time.ParseDuration(expires)
	if err != nil || ttl <= 0 {
		return c.invalid("--expires must be a positive duration such as 15m or 24h")
	}
	p, err := c.client.PresignBucketObject(context.Background(), rest[0], rest[1], strings.ToUpper(method), int(ttl.Seconds()))
	if err != nil {
		return c.fail(fmt.Errorf("presign %q: %w", rest[1], err))
	}
	return c.render(p, func() { _, _ = fmt.Fprintln(stdout, p.URL) })
}

func runBucketsSweep(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "storage buckets sweep", storageBucketsUsage(prog), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	id, ok := requireOneArg(c.fs, stderr, prog, "storage buckets sweep", "bucket id")
	if !ok {
		return exitUsage
	}
	res, err := c.client.RunBucketLifecycle(context.Background(), id)
	if err != nil {
		return c.fail(fmt.Errorf("sweep bucket %q: %w", id, err))
	}
	return c.render(res, func() { _, _ = fmt.Fprintf(stdout, "removed %d files (%d bytes)\n", res.Deleted, res.Bytes) })
}
