package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/objectstore"
)

const restoreUsage = `usage: levelrail restore --from <s3://bucket/key | s3://bucket/prefix/ | file> --identity <file> [flags]

Restores an encrypted off-box control plane backup into the data directory.
Stop the control plane first. The current database is kept as levelrail.db.pre-restore.

Flags:
  --from string          backup to restore: an s3:// key, an s3:// prefix ending in / (newest backup), or a local .db.age file
  --identity string      age identity file (the private key matching a backup recipient)
  --dry-run              download, verify and decrypt only; never touch the live database
  --force-install-id     accept a backup taken by a different install
  --endpoint string      S3 endpoint URL (default: AWS_ENDPOINT_URL, then the AWS default)
  --region string        S3 region (default: AWS_REGION, then auto)
  --path-style           use path-style S3 addressing (MinIO and most self-hosted stores)
  --json                 print the report as JSON

Credentials for s3:// sources come from AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY.
The master key is not in a backup: restore it separately from your escrow bundle.
`

// runRestore implements "levelrail restore": verify, decrypt and atomically
// install an encrypted off-box control plane backup.
func runRestore(ctx context.Context, args []string, dataDir string, stdout io.Writer, lookup func(string) (string, bool)) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	from := fs.String("from", "", "")
	identity := fs.String("identity", "", "")
	dryRun := fs.Bool("dry-run", false, "")
	force := fs.Bool("force-install-id", false, "")
	endpoint := fs.String("endpoint", "", "")
	region := fs.String("region", "", "")
	pathStyle := fs.Bool("path-style", false, "")
	asJSON := fs.Bool("json", false, "")
	if err := fs.Parse(args); err != nil {
		_, _ = fmt.Fprint(stdout, restoreUsage)
		return errors.New("invalid flags")
	}
	if *from == "" || *identity == "" {
		_, _ = fmt.Fprint(stdout, restoreUsage)
		return errors.New("--from and --identity are required")
	}
	ids, err := cpbackup.ParseIdentityFile(*identity)
	if err != nil {
		return err
	}
	src, err := restoreSource(ctx, *from, *endpoint, *region, *pathStyle, lookup)
	if err != nil {
		return err
	}
	live := filepath.Join(dataDir, storeFilename)
	if !*dryRun {
		if inUse(ctx, live) {
			return errors.New("the control plane database is in use: stop the control plane before restoring, or use --dry-run")
		}
		_, _ = fmt.Fprintln(stdout, "WARNING: the control plane must be stopped while restoring.")
	}
	rep, err := cpbackup.Restore(ctx, src, cpbackup.RestoreOptions{LivePath: live, Identities: ids, DryRun: *dryRun, ForceInstallID: *force})
	if *asJSON {
		if encErr := json.NewEncoder(stdout).Encode(rep); encErr != nil && err == nil {
			err = encErr
		}
	} else {
		printRestoreReport(stdout, rep, err == nil)
	}
	return err
}

func printRestoreReport(w io.Writer, rep cpbackup.RestoreReport, ok bool) {
	for _, c := range rep.Checks {
		_, _ = fmt.Fprintf(w, "ok      %s: %s\n", c.Name, c.Detail)
	}
	switch {
	case !ok:
	case rep.DryRun:
		_, _ = fmt.Fprintln(w, "dry run complete: the backup downloads, decrypts and passes every check. The live database was not touched.")
	default:
		if rep.PreRestore != "" {
			_, _ = fmt.Fprintf(w, "previous database kept as %s\n", rep.PreRestore)
		}
		_, _ = fmt.Fprintln(w, "restored. Make sure the master key is in place (master.key in the data directory or APP_MASTER_KEY), then start the control plane.")
	}
}

func restoreSource(ctx context.Context, from, endpoint, region string, pathStyle bool, lookup func(string) (string, bool)) (cpbackup.Source, error) {
	if !strings.HasPrefix(from, "s3://") {
		return cpbackup.NewFileSource(from), nil
	}
	u, err := url.Parse(from)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("invalid s3 URL %q", from)
	}
	get := func(k string) string { v, _ := lookup(k); return v }
	if endpoint == "" {
		endpoint = get("AWS_ENDPOINT_URL")
	}
	if region == "" {
		region = get("AWS_REGION")
	}
	if region == "" {
		region = "auto"
	}
	if get("AWS_ACCESS_KEY_ID") == "" || get("AWS_SECRET_ACCESS_KEY") == "" {
		return nil, errors.New("set AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY to read from s3://")
	}
	client, err := objectstore.New(objectstore.Config{
		Endpoint: endpoint, Region: region, Bucket: u.Host,
		AccessKeyID: get("AWS_ACCESS_KEY_ID"), SecretAccessKey: get("AWS_SECRET_ACCESS_KEY"),
		PathStyle: pathStyle || endpoint != "", HTTPClient: &http.Client{Timeout: 30 * time.Minute},
	})
	if err != nil {
		return nil, fmt.Errorf("open bucket: %w", err)
	}
	key := strings.TrimPrefix(u.Path, "/")
	if key == "" || strings.HasSuffix(key, "/") {
		if key == "" {
			key = cpbackup.Prefix + "/"
		}
		if key, err = cpbackup.LatestKey(ctx, client, key); err != nil {
			return nil, err
		}
	}
	return cpbackup.NewBucketSource(client, key), nil
}

// inUse reports whether another process holds the database open for writing.
func inUse(ctx context.Context, path string) bool {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=rw&_pragma=busy_timeout(0)")
	if err != nil {
		return false
	}
	defer func() { _ = db.Close() }()
	conn, err := db.Conn(ctx)
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, "BEGIN EXCLUSIVE"); err != nil {
		return strings.Contains(err.Error(), "locked") || strings.Contains(err.Error(), "busy")
	}
	_, _ = conn.ExecContext(ctx, "ROLLBACK")
	return false
}
