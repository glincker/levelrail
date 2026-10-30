package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const mailBackupUsageLines = `  %[1]s mail backup run [--app name] [--wait]   back up, verify and encrypt the mail server now
  %[1]s mail backup list [--app name]           list backups with size, mox version and verified state
  %[1]s mail backup verify <id> [--app name]    check a backup decrypts and matches its checksum (dry-run restore)
  %[1]s mail backup restore <id> [--app name] [--confirm <id>]   dry run, or with --confirm <id> restore for real (asks your password)
  %[1]s mail storage show [--app name]          show the backup destination
  %[1]s mail storage set --kind local|s3 [--bucket B --access-key-id K --secret-env VAR --endpoint URL --region R --path-style] [--app name]
  %[1]s mail storage test [--app name]          run the destination check again (put, get, list, delete)
`

// runMailBackup handles "mail backup run|list|verify|restore".
func runMailBackup(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(stderr, mailBackupUsageLines, prog)
		return exitUsage
	}
	sub, rest := args[0], args[1:]
	var app, confirm string
	var wait bool
	want := 0
	if sub == "verify" || sub == "restore" {
		want = 1
	}
	label := "mail backup " + sub
	c, code, ok := parseDBAccess(prog, label, label+" [flags]", rest, want, "a backup id", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&app, "app", "", "mail app when several are installed")
		switch sub {
		case "run":
			fs.BoolVar(&wait, "wait", false, "wait until the backup job finishes")
		case "restore":
			fs.StringVar(&confirm, "confirm", "", "the backup id again, to restore for real; without it this is a dry run")
		}
	})
	if !ok {
		return code
	}
	ctx := context.Background()
	switch sub {
	case "run":
		job, err := c.client.RunMailBackup(ctx, app)
		if err != nil {
			return c.fail(fmt.Errorf("mail backup run: %w", err))
		}
		if wait {
			if job, err = waitMailJob(ctx, c.client, app, job.ID); err != nil {
				return c.fail(err)
			}
		}
		return c.result(job, func() { printMailJob(stdout, job) })
	case "list":
		res, err := c.client.MailBackups(ctx, app)
		if err != nil {
			return c.fail(fmt.Errorf("mail backup list: %w", err))
		}
		return c.result(res, func() { printMailBackups(stdout, res) })
	case "verify", "restore":
		id := c.rest[0]
		if sub == "verify" || confirm == "" {
			res, err := c.client.RestoreMailBackup(ctx, app, id, "", nil)
			if err != nil {
				return c.fail(fmt.Errorf("%s %s: %w", label, id, err))
			}
			return c.result(res, func() {
				_, _ = fmt.Fprintf(stdout, "backup %s is readable: %d files, %d bytes, mox %s. Nothing was written.\n", res.BackupID, res.Files, res.Bytes, res.MoxVersion)
			})
		}
		if confirm != id {
			return c.fail(errors.New("mail backup restore: --confirm must repeat the backup id"))
		}
		res, err := withStepUp(stderr, func(p *apiclient.StepUpProof) (apiclient.MailRestoreResult, error) {
			return c.client.RestoreMailBackup(ctx, app, id, confirm, p)
		})
		if err != nil {
			return c.fail(fmt.Errorf("mail backup restore %s: %w", id, err))
		}
		if res.Job != nil {
			job, err := waitMailJob(ctx, c.client, app, res.Job.ID)
			if err != nil {
				return c.fail(err)
			}
			res.Job = &job
		}
		return c.result(res, func() { printMailJob(stdout, *res.Job) })
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown mail backup subcommand %q\n\n"+mailBackupUsageLines, prog, sub)
	return exitUsage
}

func waitMailJob(ctx context.Context, client *apiclient.Client, app, id string) (apiclient.MailBackupJob, error) {
	for {
		res, err := client.MailBackups(ctx, app)
		if err != nil {
			return apiclient.MailBackupJob{}, err
		}
		if res.Job == nil || res.Job.ID != id {
			return apiclient.MailBackupJob{}, errors.New("the job is no longer tracked, check mail backup list")
		}
		if res.Job.State != "running" {
			if res.Job.State == "failed" {
				return *res.Job, fmt.Errorf("mail %s failed at %s: %s", res.Job.Kind, res.Job.Step, res.Job.Error)
			}
			return *res.Job, nil
		}
		time.Sleep(2 * time.Second)
	}
}

func printMailJob(stdout io.Writer, j apiclient.MailBackupJob) {
	_, _ = fmt.Fprintf(stdout, "%s %s: %s (step %s)", j.Kind, j.ID, j.State, j.Step)
	if j.BackupID != "" {
		_, _ = fmt.Fprintf(stdout, ", backup %s", j.BackupID)
	}
	_, _ = fmt.Fprintln(stdout)
}

func printMailBackups(stdout io.Writer, res apiclient.MailBackups) {
	_, _ = fmt.Fprintf(stdout, "app: %s\nfolder: %s\nused: %d of %d bytes\nencrypted: %t\n\n", res.App, res.Folder, res.UsedBytes, res.CapBytes, res.Encrypted)
	tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tCREATED\tSIZE\tMOX\tVERIFIED\tWHERE")
	for _, b := range res.Backups {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%t\t%s\n", b.ID, b.CreatedAt.Format(time.RFC3339), b.StoredBytes, b.MoxVersion, b.Verified, b.Where)
	}
	_ = tw.Flush()
	if res.Job != nil {
		printMailJob(stdout, *res.Job)
	}
}

// runMailStorage handles "mail storage show|set|test".
func runMailStorage(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(stderr, mailBackupUsageLines, prog)
		return exitUsage
	}
	sub, rest := args[0], args[1:]
	var in apiclient.MailStorageInput
	var secretEnv string
	label := "mail storage " + sub
	c, code, ok := parseDBAccess(prog, label, label+" [flags]", rest, 0, "", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&in.App, "app", "", "mail app when several are installed")
		if sub == "set" {
			fs.StringVar(&in.Kind, "kind", "local", "local (the control plane data folder) or s3 (your own S3 or R2 bucket)")
			fs.StringVar(&in.Endpoint, "endpoint", "", "S3 endpoint URL, empty for AWS")
			fs.StringVar(&in.Region, "region", "", "bucket region")
			fs.StringVar(&in.Bucket, "bucket", "", "bucket name")
			fs.StringVar(&in.AccessKeyID, "access-key-id", "", "access key id")
			fs.StringVar(&secretEnv, "secret-env", "", "environment variable holding the secret access key; without it you are prompted")
			fs.BoolVar(&in.PathStyle, "path-style", false, "use path-style URLs (MinIO and most self-hosted stores)")
		}
	})
	if !ok {
		return code
	}
	ctx := context.Background()
	var res apiclient.MailStorage
	var err error
	switch sub {
	case "show":
		res, err = c.client.MailStorage(ctx, in.App)
	case "test":
		res, err = c.client.TestMailStorage(ctx, in.App)
	case "set":
		if in.Kind == "s3" {
			if secretEnv != "" {
				in.SecretAccessKey = os.Getenv(secretEnv)
			} else if in.SecretAccessKey, err = promptHidden(stderr, "Secret access key: "); err != nil {
				return c.fail(err)
			}
		}
		res, err = c.client.SetMailStorage(ctx, in)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown mail storage subcommand %q\n\n"+mailBackupUsageLines, prog, sub)
		return exitUsage
	}
	if err != nil {
		return c.fail(fmt.Errorf("%s: %w", label, err))
	}
	return c.result(res, func() { printMailStorage(stdout, res) })
}

func printMailStorage(stdout io.Writer, res apiclient.MailStorage) {
	_, _ = fmt.Fprintf(stdout, "app: %s\ndestination: %s\nlocal folder: %s (cap %d bytes)\n", res.App, res.Kind, res.Folder, res.Cap)
	if res.Bucket != nil {
		_, _ = fmt.Fprintf(stdout, "bucket: %s %s\n", res.Bucket.Bucket, res.Bucket.Endpoint)
		if !res.BucketUploads {
			_, _ = fmt.Fprintln(stdout, "note: backups are still written to the local folder in this release; the bucket is checked and saved only")
		}
	}
	steps := make([]string, 0, len(res.Steps))
	for _, s := range res.Steps {
		mark := "ok"
		if !s.OK {
			mark = "failed: " + s.Error
		}
		steps = append(steps, s.Name+" "+mark)
	}
	if len(steps) > 0 {
		_, _ = fmt.Fprintf(stdout, "check: %s\n", strings.Join(steps, ", "))
	}
}
