package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runBackupTargetsCreate implements "backup-targets create": POST
// /api/v1/backup-targets. --access-key-id/--secret-access-key are always
// required here, unlike "backup-targets update" where rotating
// credentials is optional.
func runBackupTargetsCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "backup-targets create", "print the created backup target as JSON to stdout and nothing else", stderr)
	var name, provider, endpoint, region, bucket, accessKeyID, secretAccessKey string
	fs.StringVar(&name, "name", "", "display name for the backup target (required)")
	fs.StringVar(&provider, "provider", "", "provider: aws, r2, or custom (required)")
	fs.StringVar(&endpoint, "endpoint", "", "S3-compatible endpoint URL (required for r2 and custom, aws resolves its own default)")
	fs.StringVar(&region, "region", "", "bucket region")
	fs.StringVar(&bucket, "bucket", "", "bucket name (required)")
	fs.StringVar(&accessKeyID, "access-key-id", "", "access key id (required)")
	fs.StringVar(&secretAccessKey, "secret-access-key", "", "secret access key (required)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, backupTargetsCreateUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	if name == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--name is required"))
	}
	if provider == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--provider is required"))
	}
	if bucket == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--bucket is required"))
	}
	if accessKeyID == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--access-key-id is required"))
	}
	if secretAccessKey == "" {
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

	if jsonOut {
		if err := writeJSONValue(stdout, created); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "backup target %q (id %s, provider %s) connected\n", created.Name, created.ID, created.Provider)
	return exitOK
}

func backupTargetsCreateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backup-targets create --name NAME --provider PROVIDER --bucket BUCKET --access-key-id ID --secret-access-key KEY [flags]

Connects a new backup target.

Flags:
  --name string                  display name for the backup target (required)
  --provider string               provider: aws, r2, or custom (required)
  --endpoint string               S3-compatible endpoint URL (required for r2 and custom)
  --region string                 bucket region
  --bucket string                 bucket name (required)
  --access-key-id string          access key id (required)
  --secret-access-key string      secret access key (required)
  --token string                 API token (default: %[2]s env var, then the credentials file)
  --api-url string               control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string               named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                            print the created backup target as JSON to stdout, nothing else
  -h, --help                     show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
