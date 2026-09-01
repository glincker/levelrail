package main

import (
	"context"
	"fmt"
	"io"
)

// runBackupTargetsUpdate implements "backup-targets update <id>": PUT
// /api/v1/backup-targets/{id}, a full replace of name/provider/endpoint/
// region/bucket. --access-key-id/--secret-access-key are optional here,
// unlike "backup-targets create": omitted, the target keeps its existing
// stored credentials; given together, they rotate them in place.
func runBackupTargetsUpdate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "backup-targets update", "print the updated backup target as JSON to stdout and nothing else", stderr)
	var name, provider, endpoint, region, bucket, accessKeyID, secretAccessKey string
	fs.StringVar(&name, "name", "", "display name for the backup target (required)")
	fs.StringVar(&provider, "provider", "", "provider: aws, r2, or custom (required)")
	fs.StringVar(&endpoint, "endpoint", "", "S3-compatible endpoint URL (required for r2 and custom, aws resolves its own default)")
	fs.StringVar(&region, "region", "", "bucket region")
	fs.StringVar(&bucket, "bucket", "", "bucket name (required)")
	fs.StringVar(&accessKeyID, "access-key-id", "", "new access key id; set together with --secret-access-key to rotate credentials, omit to keep the existing ones")
	fs.StringVar(&secretAccessKey, "secret-access-key", "", "new secret access key; set together with --access-key-id to rotate credentials, omit to keep the existing ones")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, backupTargetsUpdateUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "backup-targets update", "backup target id")
	if !ok {
		return exitUsage
	}

	if name == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--name is required"))
	}
	if provider == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--provider is required"))
	}
	if bucket == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--bucket is required"))
	}
	if (accessKeyID == "") != (secretAccessKey == "") {
		return reportError(stdout, stderr, jsonOut, newValidationError("--access-key-id and --secret-access-key must be set together to rotate credentials"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	updated, err := client.UpdateBackupTarget(context.Background(), id, updateBackupTargetRequest{
		Name: name, Provider: provider, Endpoint: endpoint, Region: region, Bucket: bucket,
		AccessKeyID: accessKeyID, SecretAccessKey: secretAccessKey,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("update backup target %q: %w", id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, updated); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "backup target %q (id %s) updated\n", updated.Name, updated.ID)
	return exitOK
}

func backupTargetsUpdateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backup-targets update <id> --name NAME --provider PROVIDER --bucket BUCKET [flags]

Updates a backup target's name/provider/endpoint/region/bucket. Add
--access-key-id and --secret-access-key together to rotate its stored
credentials in the same call; omit both to leave them unchanged.

Flags:
  --name string                  display name for the backup target (required)
  --provider string               provider: aws, r2, or custom (required)
  --endpoint string               S3-compatible endpoint URL (required for r2 and custom)
  --region string                 bucket region
  --bucket string                 bucket name (required)
  --access-key-id string          new access key id, set together with --secret-access-key to rotate
  --secret-access-key string      new secret access key, set together with --access-key-id to rotate
  --token string                 API token (default: %[2]s env var, then the credentials file)
  --api-url string               control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string               named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                            print the updated backup target as JSON to stdout, nothing else
  -h, --help                     show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
