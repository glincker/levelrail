package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

// Unlike update, the access key and secret are required here.
func runBackupTargetsCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "backup-targets create", "print the created backup target as JSON to stdout and nothing else", stderr)
	var name, provider, endpoint, region, bucket, accessKeyID, secretAccessKey string
	fs.StringVar(&name, "name", "", "display name for the backup target (required)")
	var targetType, localPath, caBundleFile, hostKey, privateKeyFile, passphrase string
	var trustNewHostKey bool
	fs.StringVar(&hostKey, "host-key", "", "pinned SSH host key as printed by scan-host-key, e.g. \"ssh-ed25519 SHA256:...\" (sftp only)")
	fs.BoolVar(&trustNewHostKey, "trust-new-host-key", false, "sftp: fetch the server key now, print its fingerprint and pin it without a prior check (explicit trust on first use)")
	fs.StringVar(&privateKeyFile, "private-key-file", "", "SSH private key file to authenticate with instead of a password (sftp only)")
	fs.StringVar(&passphrase, "private-key-passphrase", "", "passphrase of an encrypted private key (sftp only)")
	fs.StringVar(&caBundleFile, "ca-bundle-file", "", "PEM file of extra CA certificates to trust (webdav only)")
	fs.StringVar(&provider, "provider", "", "provider: aws, r2, custom, local, webdav or sftp (required)")
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
	if provider == "sftp" && bucket == "" {
		bucket = localPath
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
	if provider != "local" && provider != "sftp" && bucket == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--bucket is required"))
	}
	if msg := backupTargetProviderProblem(provider, endpoint, region); msg != "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("%s", msg))
	}
	if provider != "local" && accessKeyID == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--access-key-id is required"))
	}
	if provider == "sftp" {
		if (secretAccessKey == "") == (privateKeyFile == "") {
			return reportError(stdout, stderr, jsonOut, newValidationError("sftp needs exactly one of --secret-access-key (password) or --private-key-file"))
		}
		if (hostKey == "") == !trustNewHostKey {
			return reportError(stdout, stderr, jsonOut, newValidationError("sftp needs exactly one of --host-key (verified) or --trust-new-host-key (pin what the server shows now)"))
		}
	}
	if provider != "local" && provider != "sftp" && secretAccessKey == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--secret-access-key is required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	caBundle := ""
	if caBundleFile != "" {
		raw, rerr := os.ReadFile(caBundleFile) //nolint:gosec // operator chosen CA bundle path
		if rerr != nil {
			return reportError(stdout, stderr, jsonOut, newValidationError("read --ca-bundle-file: %v", rerr))
		}
		caBundle = string(raw)
	}
	privateKey := ""
	if privateKeyFile != "" {
		raw, rerr := os.ReadFile(privateKeyFile) //nolint:gosec // operator chosen key path
		if rerr != nil {
			return reportError(stdout, stderr, jsonOut, newValidationError("read --private-key-file: %v", rerr))
		}
		privateKey = string(raw)
	}
	if trustNewHostKey {
		scan, serr := client.ScanSFTPHostKey(context.Background(), endpoint)
		if serr != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("scan sftp host key: %w", serr))
		}
		_, _ = fmt.Fprintf(stderr, "pinning host key %s\n", scan.HostKey)
		hostKey = scan.HostKey
	}
	created, err := client.CreateBackupTarget(context.Background(), createBackupTargetRequest{
		Name: name, Provider: provider, Endpoint: endpoint, Region: region, Bucket: bucket,
		AccessKeyID: accessKeyID, SecretAccessKey: secretAccessKey, CABundle: caBundle,
		PrivateKey: privateKey, PrivateKeyPassphrase: passphrase, HostKey: hostKey,
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
	case "webdav":
		if !strings.HasPrefix(endpoint, "https://") {
			return "--endpoint must be an https URL for the webdav provider (the bucket is the remote directory, key id and secret are the username and password)"
		}
	case "sftp":
		if endpoint == "" {
			return "--endpoint is required for the sftp provider (HOST or HOST:PORT; --path is the remote directory, key id is the SSH username)"
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
  --provider string               provider: aws, r2, custom, local, webdav or sftp (required; --type is an alias)
  --path string                   directory under APP_BACKUP_LOCAL_ROOT, default APP_DATA_DIR/backups (local type, replaces bucket and keys; lives on the server, so it does not survive losing it)
  --endpoint string               S3-compatible endpoint URL (required for r2 and custom)
  --region string                 bucket region (required for custom)
  --bucket string                 bucket name (required)
  --access-key-id string          access key id (required)
  --secret-access-key string      secret access key (required)
  --ca-bundle-file string         PEM file of extra CA certificates to trust (webdav only)
  --host-key string               pinned SSH host key from scan-host-key (sftp; or use --trust-new-host-key)
  --trust-new-host-key            sftp: pin the key the server shows now and print it (explicit trust on first use)
  --private-key-file string       SSH private key file instead of a password (sftp)
  --private-key-passphrase string passphrase of an encrypted private key (sftp)
  --token string                 API token (default: %[2]s env var, then the credentials file)
  --api-url string               control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string               named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                            print the created backup target as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}
