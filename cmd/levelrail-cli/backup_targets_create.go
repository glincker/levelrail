package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/GLINCKER/levelrail/internal/fielddesc"
)

// Unlike update, the access key and secret are required here.
func runBackupTargetsCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "backup-targets create", "print the created backup target as JSON to stdout and nothing else", stderr)
	var targetType string
	var trustNewHostKey bool
	fs.StringVar(&targetType, "type", "", "alias of --provider")
	fs.BoolVar(&trustNewHostKey, "trust-new-host-key", false, "sftp: fetch the server key now, print its fingerprint and pin it without a prior check (explicit trust on first use)")
	// Flags come from the field descriptors so a new field needs no edit here.
	descFlags := registerDescriptorFlags(fs, fielddesc.BackupTarget)
	val := func(flagName string) string { return *descFlags[flagName] }
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, backupTargetsCreateUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	name, provider, endpoint, region, bucket := val("name"), val("provider"), val("endpoint"), val("region"), val("bucket")
	accessKeyID, secretAccessKey, hostKey, passphrase := val("access-key-id"), val("secret-access-key"), val("host-key"), val("private-key-passphrase")
	caBundleFile, privateKeyFile, localPath := val("ca-bundle-file"), val("private-key-file"), val("path")
	if provider == "" {
		provider = targetType
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
	if msg := backupTargetProviderProblem(provider, endpoint, region); msg != "" && name != "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("%s", msg))
	}
	if provider == "sftp" {
		if (secretAccessKey == "") == (privateKeyFile == "") {
			return reportError(stdout, stderr, jsonOut, newValidationError("sftp needs exactly one of --secret-access-key (password) or --private-key-file"))
		}
		if (hostKey == "") == !trustNewHostKey {
			return reportError(stdout, stderr, jsonOut, newValidationError("sftp needs exactly one of --host-key (verified) or --trust-new-host-key (pin what the server shows now)"))
		}
	}
	if msg := backupTargetDescriptorProblem(map[string]string{
		"name": name, "provider": provider, "endpoint": endpoint, "region": region, "bucket": bucket,
		"access_key_id": accessKeyID, "secret_access_key": secretAccessKey, "private_key": privateKeyFile,
		"host_key": hostKey,
	}, trustNewHostKey); msg != "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("%s", msg))
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
  --output string          output format: json, ndjson, yaml, table, wide, text or name (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}

// registerDescriptorFlags defines one string flag per descriptor flag and returns the
// destination of each by flag name. Bool flags are registered too and read with fs.Lookup.
func registerDescriptorFlags(fs *flag.FlagSet, resource string) map[string]*string {
	return registerDescriptorFlagsWhen(fs, resource, nil)
}

// registerDescriptorFlagsWhen limits the flags to the fields visible for values, so a per
// provider verb gets only that provider's flags.
func registerDescriptorFlagsWhen(fs *flag.FlagSet, resource string, values map[string]string) map[string]*string {
	out := map[string]*string{}
	res, ok := fielddesc.Get(resource)
	if !ok {
		return out
	}
	specs := res.FlagSpecs()
	if values != nil {
		specs = res.FlagSpecsWhen(values)
	}
	for _, f := range specs {
		if f.Bool {
			fs.Bool(f.Name, false, f.Usage)
			continue
		}
		if f.Multi {
			dst := new(string)
			fs.Var(joinedFlag{dst}, f.Name, f.Usage)
			out[f.Name] = dst
			continue
		}
		out[f.Name] = fs.String(f.Name, "", f.Usage)
	}
	return out
}

// joinedFlag lets a descriptor flag repeat: every value is appended to one comma list.
type joinedFlag struct{ dst *string }

func (j joinedFlag) String() string {
	if j.dst == nil {
		return ""
	}
	return *j.dst
}

func (j joinedFlag) Set(v string) error {
	if *j.dst != "" {
		*j.dst += ","
	}
	*j.dst += v
	return nil
}

// backupTargetDescriptorProblem runs the shared field registry and words the first problem
// with the CLI flag name, so required and select errors match the API apart from the dashes.
func backupTargetDescriptorProblem(values map[string]string, hostKeyTrusted bool) string {
	res, ok := fielddesc.Get(fielddesc.BackupTarget)
	if !ok {
		return ""
	}
	if values["private_key"] != "" {
		values["auth"] = "key"
	} else {
		values["auth"] = "password"
	}
	if hostKeyTrusted && values["host_key"] == "" {
		values["host_key"] = "pending"
	}
	problems := res.Validate(values)
	if len(problems) == 0 {
		return ""
	}
	msg := problems[0].Message
	for _, f := range res.FlagSpecs() {
		if f.Key == problems[0].Key {
			return strings.Replace(msg, f.Key, "--"+f.Name, 1)
		}
	}
	return msg
}
