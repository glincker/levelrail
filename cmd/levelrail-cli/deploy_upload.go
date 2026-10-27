package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/sourceupload"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/kit/untrusted"
)

const (
	envUploadMaxBytes       = "APP_UPLOAD_MAX_BYTES"
	defaultUploadMaxBytes   = int64(512 << 20)
	uploadTopFiles          = 10
	defaultDeployUploadWait = "15m"
)

// deployUploadResult is the --json shape and the data behind the final summary.
type deployUploadResult struct {
	App      string `json:"app"`
	Created  bool   `json:"created"`
	Hash     string `json:"hash,omitempty"`
	Files    int    `json:"files"`
	Bytes    int64  `json:"bytes"`
	Archive  int64  `json:"archive_bytes,omitempty"`
	Deduped  bool   `json:"deduped,omitempty"`
	DeployID string `json:"deploy_id,omitempty"`
	Status   string `json:"status,omitempty"`
	URL      string `json:"url,omitempty"`
	// Certificate is issued, pending or failed for the primary domain; empty for a fallback URL or when unknown.
	Certificate string `json:"certificate,omitempty"`
	// Failure is the server's classified reason when the build or rollout failed.
	Failure  *apiclient.DeployFailure `json:"failure,omitempty"`
	Error    string                   `json:"error,omitempty"`
	DryRun   bool                     `json:"dry_run,omitempty"`
	FileList []string                 `json:"file_list,omitempty"`
}

// wantsUploadDeploy reports whether "deploy" args mean a local directory rather than a git or image deploy.
func wantsUploadDeploy(args []string) bool {
	if len(args) > 0 && args[0] == "status" {
		return false
	}
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		return false
	}
	for _, a := range args {
		name := strings.TrimLeft(strings.SplitN(a, "=", 2)[0], "-")
		switch name {
		case "image", "commit", "latest", "branch", "from-cache", "diff", "confirm":
			if strings.HasPrefix(a, "-") {
				return false
			}
		}
	}
	return true
}

func uploadMaxBytes(lookupEnv func(string) (string, bool)) (int64, error) {
	v, ok := lookupEnv(envUploadMaxBytes)
	if !ok || strings.TrimSpace(v) == "" {
		return defaultUploadMaxBytes, nil
	}
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil || n <= 0 {
		return 0, newValidationError("%s must be a positive number of bytes, got %q", envUploadMaxBytes, v)
	}
	return n, nil
}

// runDeployUpload implements "deploy [path]": pack the directory, upload it, build it and follow the deploy.
func runDeployUpload(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, tokenP, apiURLP, profileP, jsonOutP, outputP, queryP := apiFlagSet(prog, "deploy", "print the result as JSON to stdout and nothing else", stderr)
	var appName, name, fromHash, timeoutStr string
	var port int
	var dryRun, yes, static, waitURL bool
	env, buildEnv := stringMapFlag{}, stringMapFlag{}
	var include []string
	fs.StringVar(&appName, "app", "", "app to deploy to (default: the directory name, created on the first deploy)")
	fs.StringVar(&name, "name", "", "same as --app")
	fs.IntVar(&port, "port", 0, "container port (default: the app spec, the Dockerfile or the detected framework)")
	fs.Var(env, "env", "runtime env var as KEY=VALUE, repeatable (stored on the app, visible in its env list)")
	fs.Var(buildEnv, "build-env", "Dockerfile build argument as KEY=VALUE, repeatable")
	fs.Func("include", "upload a path the ignore rules would skip, for example .env; repeatable", func(s string) error {
		include = append(include, s)
		return nil
	})
	fs.StringVar(&fromHash, "upload-hash", "", "rebuild an earlier upload that is still stored, without sending the directory again")
	fs.StringVar(&timeoutStr, "timeout", defaultDeployUploadWait, "stop following the deploy after this long (the deploy itself keeps running)")
	fs.BoolVar(&static, "static", false, "publish a folder or .zip as a static site: no build, no source control (implied by a .zip argument)")
	fs.BoolVar(&waitURL, "wait-url", false, "after a healthy deploy, wait for the domain certificate to be issued (limit: "+envDeployURLWait+")")
	fs.BoolVar(&dryRun, "dry-run", false, "list the files that would be uploaded and change nothing")
	fs.BoolVar(&yes, "yes", false, "do not ask for confirmation on a terminal")
	fs.BoolVar(&yes, "y", false, "shorthand for --yes")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, deployUploadUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonOutP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	dir := "."
	switch rest := fs.Args(); len(rest) {
	case 0:
	case 1:
		dir = rest[0]
	default:
		_, _ = fmt.Fprintf(stderr, "%s: deploy takes at most one directory\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	staticDrop := static || isZipPath(dir)
	if st, err := os.Stat(dir); !staticDrop && (err != nil || !st.IsDir()) {
		return reportError(stdout, stderr, jsonOut, newValidationError("%q is not a directory; to deploy an image use \"%s deploy APP --image IMAGE\"", dir, prog))
	}
	if appName == "" {
		appName = name
	}
	timeout, err := parseDurationOrZero(timeoutStr)
	if err != nil || timeout <= 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--timeout must be a valid positive duration, got %q", timeoutStr))
	}
	maxBytes, err := uploadMaxBytes(lookupEnv)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	var urlWait time.Duration
	if waitURL {
		if urlWait, err = deployURLWait(lookupEnv); err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
	}

	if staticDrop {
		opts := staticOpts{appName: appName, fromHash: fromHash, include: include, timeout: timeout, maxBytes: maxBytes, dryRun: dryRun, yes: yes}
		newClient := func() *Client { return apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv) }
		return runDeployStatic(prog, dir, opts, newClient, stdout, stderr, of, jsonOut, func() bool {
			return !isTerminal(stdin) || confirmUp(stdin, stderr)
		})
	}
	if appName == "" {
		appName = dirAppName(dir)
	}
	inf, err := inferUp(dir, upFlags{name: appName, port: port, upload: true}, gitSource{})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	if err := checkUploadBuildType(inf); err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	say := func(format string, a ...any) {
		if !jsonOut {
			_, _ = fmt.Fprintf(stderr, format+"\n", a...)
		}
	}
	ctx := context.Background()
	res := deployUploadResult{App: inf.Flags.name, DryRun: dryRun}

	if fromHash == "" {
		plan, err := sourceupload.Scan(ctx, dir, sourceupload.Options{Include: include, MaxBytes: maxBytes})
		if err != nil {
			return reportError(stdout, stderr, jsonOut, uploadScanError(err, maxBytes))
		}
		if len(plan.Files) == 0 {
			return reportError(stdout, stderr, jsonOut, newValidationError("nothing to upload in %s: every file is ignored", dir))
		}
		res.Files, res.Bytes = len(plan.Files), plan.Total
		printUploadPlan(stderr, jsonOut, inf, plan, dryRun)
		if dryRun {
			for _, f := range plan.Files {
				res.FileList = append(res.FileList, f.Path)
			}
			return writeScheduledTaskResult(stdout, stderr, of, res, func() {
				for _, p := range res.FileList {
					_, _ = fmt.Fprintln(stdout, p)
				}
			})
		}
		if !jsonOut && !yes && isTerminal(stdin) && !confirmUp(stdin, stderr) {
			_, _ = fmt.Fprintln(stderr, "cancelled, nothing uploaded")
			return exitUsage
		}
		client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
		if err := ensureUploadApp(ctx, client, inf, env, &res, say); err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		hash, err := packAndUpload(ctx, client, inf.Flags.name, plan, maxBytes, &res, stderr, jsonOut)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		fromHash = hash
		return finishUploadDeploy(ctx, client, prog, inf, fromHash, buildEnv, timeout, urlWait, &res, stdout, stderr, of, jsonOut)
	}

	if !validUploadHash(fromHash) {
		return reportError(stdout, stderr, jsonOut, newValidationError("--upload-hash must be a 64 character hex digest"))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if dryRun {
		say("would rebuild upload %s for app %q", fromHash[:12], res.App)
		return writeScheduledTaskResult(stdout, stderr, of, res, func() {})
	}
	if err := ensureUploadApp(ctx, client, inf, env, &res, say); err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	res.Hash = fromHash
	return finishUploadDeploy(ctx, client, prog, inf, fromHash, buildEnv, timeout, urlWait, &res, stdout, stderr, of, jsonOut)
}

// dirAppName turns the directory's own name into a valid app name, or "" to fall back to detection.
func dirAppName(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	return strings.Trim(dbNamePattern.ReplaceAllString(strings.ToLower(filepath.Base(abs)), "-"), "-")
}

// validUploadHash reports whether s is a sha256 hex digest.
func validUploadHash(s string) bool { return sourceupload.ValidHash(s) }

func checkUploadBuildType(inf upInference) error {
	switch inf.Flags.buildType {
	case spec.BuildDockerfile, spec.BuildRailpack:
		return nil
	case "":
		if inf.FileSpec != nil {
			return newValidationError("app.yaml services are not supported by a directory deploy yet, use a single Dockerfile or detected app")
		}
	}
	return newValidationError("a directory deploy builds with a Dockerfile or Railpack, this directory is detected as %q; add a Dockerfile to deploy it", inf.Flags.buildType)
}

func uploadScanError(err error, maxBytes int64) error {
	if errors.Is(err, sourceupload.ErrTooLarge) {
		return newValidationError("the directory is larger than the %s upload limit; exclude build output or dependencies in .gitignore, or raise %s", humanBytes(maxBytes), envUploadMaxBytes)
	}
	return fmt.Errorf("scan directory: %w", err)
}

func printUploadPlan(w io.Writer, quiet bool, inf upInference, plan sourceupload.Plan, dryRun bool) {
	if quiet {
		return
	}
	_, _ = fmt.Fprintf(w, "deploy: %s as app %q (%s)\n", plan.Root, inf.Flags.name, inf.Stack.Label)
	_, _ = fmt.Fprintf(w, "  %d files, %s to upload (.gitignore, node_modules, .git and .env files are skipped)\n", len(plan.Files), humanBytes(plan.Total))
	_, _ = fmt.Fprintln(w, "  largest files:")
	for _, f := range plan.Largest(uploadTopFiles) {
		_, _ = fmt.Fprintf(w, "    %10s  %s\n", humanBytes(f.Size), f.Path)
	}
	if dryRun {
		_, _ = fmt.Fprintln(w, "  dry run: nothing is sent, the full list follows on stdout")
	}
}

func ensureUploadApp(ctx context.Context, client *Client, inf upInference, env map[string]string, res *deployUploadResult, say func(string, ...any)) error {
	name := inf.Flags.name
	existing, exists, err := lookupApp(ctx, client, name)
	if err != nil {
		return fmt.Errorf("look up app %q: %w", name, err)
	}
	res.Created = !exists
	if !exists {
		body := appResource{Name: name, Image: pendingImageTag(name), Port: inf.Flags.port}
		if len(env) > 0 {
			body.Env = map[string]string(env)
		}
		if _, err := client.CreateApp(ctx, body); err != nil {
			return fmt.Errorf("create app %q: %w", name, err)
		}
		say("created app %q on port %d", name, inf.Flags.port)
		return nil
	}
	if len(env) == 0 {
		return nil
	}
	merged := map[string]string{}
	for k, v := range existing.Env {
		merged[k] = v
	}
	for k, v := range env {
		merged[k] = v
	}
	existing.Env = merged
	if _, err := client.UpdateApp(ctx, name, existing); err != nil {
		return fmt.Errorf("update env of app %q: %w", name, err)
	}
	say("updated %d env var(s) on app %q", len(env), name)
	return nil
}

func packAndUpload(ctx context.Context, client *Client, app string, plan sourceupload.Plan, maxBytes int64, res *deployUploadResult, stderr io.Writer, quiet bool) (string, error) {
	tmp, err := os.CreateTemp("", "upload-*.tar.gz")
	if err != nil {
		return "", fmt.Errorf("create temp archive: %w", err)
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	if err := plan.WriteArchive(ctx, tmp); err != nil {
		return "", fmt.Errorf("pack directory: %w", err)
	}
	size, err := tmp.Seek(0, io.SeekCurrent)
	if err != nil {
		return "", fmt.Errorf("measure archive: %w", err)
	}
	if size > maxBytes {
		return "", newValidationError("the compressed archive is %s, over the %s upload limit; raise %s or trim the directory", humanBytes(size), humanBytes(maxBytes), envUploadMaxBytes)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("rewind archive: %w", err)
	}
	res.Archive = size
	var lastStep int64
	progress := func(sent int64) {
		if quiet || size == 0 {
			return
		}
		if step := sent * 4 / size; step > lastStep {
			lastStep = step
			_, _ = fmt.Fprintf(stderr, "  uploading %d%% (%s of %s)\n", step*25, humanBytes(sent), humanBytes(size))
		}
	}
	up, err := client.UploadSource(ctx, app, tmp, size, progress)
	if err != nil {
		return "", fmt.Errorf("upload to app %q: %w", app, err)
	}
	res.Hash, res.Deduped = up.Hash, up.Deduped
	if !quiet {
		note := ""
		if up.Deduped {
			note = ", identical to an earlier upload"
		}
		_, _ = fmt.Fprintf(stderr, "  uploaded %s as %s%s\n", humanBytes(size), up.Hash[:12], note)
	}
	return up.Hash, nil
}

// logDrainGrace lets the last log lines print after the verdict before the follow is cut.
const logDrainGrace = 2 * time.Second

func finishUploadDeploy(ctx context.Context, client *Client, prog string, inf upInference, hash string, buildEnv map[string]string, timeout, urlWait time.Duration, res *deployUploadResult, stdout, stderr io.Writer, of outputFlags, jsonOut bool) int {
	name := inf.Flags.name
	req := buildTriggerRequest{
		UploadHash: hash,
		ImageRepo:  name,
		Build:      buildTriggerRequestBuild{Type: inf.Flags.buildType, Path: inf.Flags.dockerfile},
	}
	if inf.Flags.buildType == spec.BuildDockerfile && len(buildEnv) > 0 {
		req.Build.Args = map[string]string(buildEnv)
	} else if len(buildEnv) > 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--build-env only applies to Dockerfile builds"))
	}
	if inf.Flags.buildType == spec.BuildRailpack {
		req.Build.Path = ""
	}
	trig, err := client.TriggerBuild(ctx, name, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("app %q is ready but the build did not start, re-run \"%s deploy --upload-hash %s --app %s\" to retry: %w", name, prog, hash, name, err))
	}
	res.DeployID = trig.ID
	var logDone chan struct{}
	stopLog := func() {}
	if !jsonOut {
		if trig.Status == "queued" {
			_, _ = fmt.Fprintf(stderr, "queued behind another deploy (%s)\n", trig.WaitReason)
		}
		_, _ = fmt.Fprintf(stderr, "building %q (deploy %s)\n", name, trig.ID)
		followCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		logDone = make(chan struct{})
		stopLog = cancel
		go func() {
			defer close(logDone)
			_ = client.StreamDeployLog(followCtx, name, trig.ID, func(e apiclient.LogStreamEntry) error {
				_, _ = fmt.Fprintln(stderr, e.Line)
				return nil
			})
		}()
	}
	wait, werr := client.WaitForDeploy(ctx, name, trig.ID, apiclient.WaitOptions{Interval: 2 * time.Second, Window: timeout})
	// The log stream stays open after the deploy ends, so stop following once the verdict is in.
	if logDone != nil {
		select {
		case <-logDone:
		case <-time.After(logDrainGrace):
		}
		stopLog()
		<-logDone
	}
	if werr != nil && !errors.Is(werr, context.Canceled) {
		return reportError(stdout, stderr, jsonOut, werr)
	}
	res.Status = wait.Deploy.Outcome
	if !wait.Done {
		res.Status = "timeout"
	}
	res.Failure, res.Error = wait.Deploy.Failure, wait.Deploy.Error
	if final, err := client.GetApp(ctx, name); err == nil {
		res.URL = appPrimaryURL(final)
		if domain := primaryDomain(final); domain != "" && res.Status == apiclient.DeployOutcomeHealthy {
			res.Certificate = domainCertState(ctx, client, domain)
			if res.Certificate == certPending && urlWait > 0 {
				if !jsonOut {
					_, _ = fmt.Fprintln(stderr, "waiting for the certificate on "+res.URL)
				}
				res.Certificate = waitForDomainCert(ctx, client, domain, urlWait)
			}
		}
	}
	code := writeScheduledTaskResult(stdout, stderr, of, res, func() {
		if res.Status == apiclient.DeployOutcomeHealthy && res.URL != "" {
			_, _ = fmt.Fprintln(stdout, res.URL)
			if res.Certificate == certIssued && urlWait > 0 {
				_, _ = fmt.Fprintln(stderr, "certificate issued, "+res.URL+" is ready")
			} else if note := certNote(res.URL, res.Certificate); note != "" {
				_, _ = fmt.Fprintln(stderr, note)
			}
		}
		_, _ = fmt.Fprintf(stderr, "deploy %s: %s\n", res.DeployID, res.Status)
		printUploadFailure(stderr, res)
	})
	if code != exitOK {
		return code
	}
	switch {
	case !wait.Done:
		return exitDeployTimeout
	case res.Status == apiclient.DeployOutcomeHealthy:
		return exitOK
	default:
		return exitDeployNotHealthy
	}
}

func deployUploadUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s deploy [dir] [flags]

Uploads a directory from this computer, builds it on the control plane and
follows the deploy. No git remote and no registry needed. The first deploy
creates the app (name and port are inferred, override with --app and --port);
later deploys update it. Files matching .gitignore, plus node_modules, .git,
build caches and .env or key files, are never uploaded unless --include names
them. The file count and the largest files are shown before anything is sent.

Static sites need no build: "%[1]s deploy ./site --static" or "%[1]s deploy site.zip"
publishes the folder or zip as is (_redirects, _headers and a 404.html are honored).
A folder or zip wrapped in one top-level directory is unwrapped for you.

To deploy a git repository by URL, use "%[1]s deploy <git-url>".
To deploy an image or a git commit instead, use "%[1]s deploy APP --image IMAGE"
or "--latest".

Flags:
  --app string          app name (default: the directory name)
  --name string         same as --app
  --port int            container port (default: Dockerfile EXPOSE or detected framework)
  --env KEY=VALUE       runtime env var, repeatable
  --build-env KEY=VALUE Dockerfile build argument, repeatable
  --include PATH        upload a normally ignored path, repeatable
  --upload-hash HASH    rebuild an earlier upload that is still stored (default retention 7 days)
  --timeout duration    stop following the deploy after this long (default %[2]s)
  --static              publish as a static site, no build (implied for a .zip)
  --wait-url            after a healthy deploy, wait for the domain certificate (limit %[4]s, default %[5]s)
  --dry-run             list the files that would be uploaded, send nothing
  --yes, -y             do not ask for confirmation on a terminal
  --token, --api-url, --profile, --json, --output, --query as for every command

Size limit: %[3]s (default 512 MiB, compressed), also enforced by the server.

Example:
  %[1]s deploy ./my-app --env LOG_LEVEL=info
`, prog, defaultDeployUploadWait, envUploadMaxBytes, envDeployURLWait, defaultDeployURLWait)
}

// printUploadFailure shows why a deploy did not succeed; the server already
// redacts and caps both the failure excerpt and the error text.
func printUploadFailure(w io.Writer, res *deployUploadResult) {
	switch {
	case res.Failure != nil:
		printDeployFailure(w, res.Failure)
	case res.Error != "":
		// The server strips control codes too; an older one may not.
		_, _ = fmt.Fprintf(w, "reason: %s\n", untrusted.Clean(res.Error))
	}
}
