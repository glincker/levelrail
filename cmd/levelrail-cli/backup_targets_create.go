package main

import (
	"context"
	"fmt"
	"io"
)

// Unlike update, the access key and secret are required here.
func runBackupTargetsCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "backup-targets create", "print the created backup target as JSON to stdout and nothing else", stderr)
	var name, provider, endpoint, region, bucket, accessKeyID, secretAccessKey string
	fs.StringVar(&name, "name", "", "display name for the backup target (required)")
	var targetType, localPath string
	fs.StringVar(&provider, "provider", "", "provider: aws, r2, custom, or local (required)")
	fs.StringVar(&targetType, "type", "", "alias of --provider")
	fs.StringVar(&localPath, "path", "", "directory under APP_BACKUP_LOCAL_ROOT (local type only)")
	fs.StringVar(&endpoint, "endpoint", "", "S3-compatible endpoint URL (required for r2 and custom, aws resolves its own default)")
	fs.StringVar(&region, "region", "", "bucket region")
	fs.StringVar(&bucket, "bucket", "", "bucket name (required)")
	fs.StringVar(&accessKeyID, "access-key-id", "", "access key id (required)")
	fs.StringVar(&secretAccessKey, "secret-access-key", "", "secret access key (required)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, backupTargetsCreateUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	if provider == "" {
		provider = targetType
	}
	if name == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--name is required"))
	}
	if provider == "local" {
		if localPath == "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("--path is required for the local type"))
		}
		bucket = localPath
	}
	if provider == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--provider is required"))
	}
	if provider != "local" && bucket == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--bucket is required"))
	}
	if msg := backupTargetProviderProblem(provider, endpoint, region); msg != "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("%s", msg))
	}
	if provider != "local" && accessKeyID == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--access-key-id is required"))
	}
	if provider != "local" && secretAccessKey == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--secret-access-key is required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	created, err := client.CreateBackupTarget(context.Background(), createBackupTargetRequest{
		Name: name, Provider: provider, Endpoint: endpoint, Region: region, Bucket: bucket,
		AccessKeyID: accessKeyID, SecretAccessKey: secretAccessKey,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create backup target %q: %w", name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, created, func() {
		_, _ = fmt.Fprintf(stdout, "backup target %q (id %s, provider %s) connected\n", created.Name, created.ID, created.Provider)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// backupTargetProviderProblem mirrors the API's per-provider required fields
// so a bad create fails before a request is sent.
func backupTargetProviderProblem(provider, endpoint, region string) string {
	switch provider {
	case "r2":
		if endpoint == "" {
			return "--endpoint is required for the r2 provider"
		}
	case "custom":
		if endpoint == "" {
			return "--endpoint is required for the custom provider"
		}
		if region == "" {
			return "--region is required for the custom provider"
		}
	}
	return ""
}

func backupTargetsCreateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backup-targets create --name NAME --provider PROVIDER --bucket BUCKET --access-key-id ID --secret-access-key KEY [flags]

Connects a new backup target.

Flags:
  --name string                  display name for the backup target (required)
  --provider string               provider: aws, r2, custom, or local (required; --type is an alias)
  --path string                   directory under APP_BACKUP_LOCAL_ROOT, default APP_DATA_DIR/backups (local type, replaces bucket and keys; lives on the server, so it does not survive losing it)
  --endpoint string               S3-compatible endpoint URL (required for r2 and custom)
  --region string                 bucket region (required for custom)
  --bucket string                 bucket name (required)
  --access-key-id string          access key id (required)
  --secret-access-key string      secret access key (required)
  --token string                 API token (default: %[2]s env var, then the credentials file)
  --api-url string               control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string               named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                            print the created backup target as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
