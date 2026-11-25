package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/agentupdate"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/cliupdate"
	"github.com/GLINCKER/levelrail/internal/nodeinsight"
	"github.com/GLINCKER/levelrail/internal/selfupgrade"
	"github.com/GLINCKER/levelrail/internal/version"
)

const (
	envServiceName    = "APP_AGENT_SERVICE_NAME"
	envUpdateInterval = "APP_AGENT_UPDATE_CHECK_SECONDS"
	envReleaseBase    = "LEVELRAIL_RELEASE_BASE_URL"
	envControlPlane   = "APP_CONTROL_PLANE_ADDR"
	updateHTTPTimeout = 5 * time.Minute
	smokeTimeout      = 10 * time.Second
	// exitPackageManaged is the self-update exit code when a package manager owns the binary.
	exitPackageManaged = 3
)

// updateConfigured says whether this agent knows its control plane address.
// The check itself is authenticated by the node's own mTLS identity.
func updateConfigured(lookup func(string) (string, bool)) bool {
	v, ok := lookup(envControlPlane)
	return ok && v != ""
}

// identityDecider asks the agent listener with the enrolled node identity,
// so the answer is scoped to this node and no API token exists on the host.
func identityDecider(lookup func(string) (string, bool)) agentupdate.Decider {
	addr, _ := lookup(envControlPlane)
	return func(ctx context.Context) (nodeinsight.UpdateDecision, error) {
		id, err := agent.NewIdentityFile(identityFilePath()).Load()
		if err != nil {
			return nodeinsight.UpdateDecision{}, fmt.Errorf("load node identity: %w", err)
		}
		return agent.UpdateCheckAt(ctx, addr, id)
	}
}

// signedRelease fetches from the same release source as the control plane
// self-upgrade and always verifies the signature in require mode.
type signedRelease struct {
	fetcher selfupgrade.Fetcher
	signer  selfupgrade.Signer
}

func (r signedRelease) Fetch(ctx context.Context, tag, asset, dir string) (selfupgrade.Fetched, error) {
	return r.fetcher.Fetch(ctx, tag, asset, dir)
}

func (r signedRelease) Verify(ctx context.Context, checksums, bundle string) (string, error) {
	return r.signer.Verify(ctx, checksums, bundle)
}

func newSignedRelease(lookup func(string) (string, bool)) (signedRelease, string, error) {
	b, err := brand.LoadEmbedded()
	if err != nil {
		return signedRelease{}, "", fmt.Errorf("load release settings: %w", err)
	}
	repo := b.RepoSlug()
	if repo == "" {
		return signedRelease{}, "", errors.New("no release repository is configured")
	}
	base := "https://github.com/" + repo + "/releases/download"
	if v, ok := lookup(envReleaseBase); ok && v != "" {
		base = v
	}
	return signedRelease{
		fetcher: selfupgrade.Fetcher{BaseURL: base, Client: &http.Client{Timeout: updateHTTPTimeout}, RedirectHosts: redirectHosts(base)},
		signer: selfupgrade.Signer{
			Mode: selfupgrade.VerifyRequire, IdentityRegexp: selfupgrade.ReleaseIdentityRegexp(repo),
			Issuer: selfupgrade.CosignOIDCIssuer, TUFCacheDir: filepath.Join(agentDataDir(), "tuf"),
		},
	}, cliupdate.Asset(b.BinaryName+"-agent", runtime.GOOS, runtime.GOARCH), nil
}

// redirectHosts limits release redirects to the GitHub download hosts and the
// configured mirror host, so an asset cannot bounce the agent inward.
func redirectHosts(base string) []string {
	hosts := []string{"github.com", "githubusercontent.com"}
	if u, err := url.Parse(base); err == nil && u.Hostname() != "" {
		hosts = append(hosts, u.Hostname())
	}
	return hosts
}

func systemService(lookup func(string) (string, bool)) agentupdate.SystemService {
	name := agentupdate.DefaultServiceName(runtime.GOOS)
	if v, ok := lookup(envServiceName); ok && v != "" {
		name = v
	}
	return agentupdate.SystemService{Name: name}
}

// smokeVersion runs the staged binary and returns the version it reports.
func smokeVersion(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, smokeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version") //nolint:gosec // staged, checksum-verified binary, fixed argv
	// No inherited environment (it carries enrollment secrets) and a neutral working directory.
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	if runtime.GOOS == "windows" {
		cmd.Env = []string{"SystemRoot=" + os.Getenv("SystemRoot")}
	}
	cmd.Dir = filepath.Dir(path)
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// buildUpdateOptions wires one run. allowPackageManaged is only ever set by an
// explicit manual flag; the automatic loop always passes false.
func buildUpdateOptions(lookup func(string) (string, bool), allowPackageManaged bool) (agentupdate.Options, error) {
	if !updateConfigured(lookup) {
		return agentupdate.Options{}, fmt.Errorf("set %s so the agent can ask the control plane for its target version", envControlPlane)
	}
	rel, asset, err := newSignedRelease(lookup)
	if err != nil {
		return agentupdate.Options{}, err
	}
	exe, err := cliupdate.ExecutablePath()
	if err != nil {
		return agentupdate.Options{}, err
	}
	if !allowPackageManaged && agentupdate.PackageManaged(exe, agentDataDir()) {
		return agentupdate.Options{}, fmt.Errorf("%w (%s)", agentupdate.ErrPackageManaged, exe)
	}
	return agentupdate.Options{
		Decide: identityDecider(lookup), Release: rel, Service: systemService(lookup),
		Asset: asset, Current: version.Version, ExePath: exe, Smoke: smokeVersion, DataDir: agentDataDir(),
	}, nil
}

// runSelfUpdate implements `levelrail-agent self-update [--check|--apply]`.
func runSelfUpdate(args []string, out, errOut io.Writer, lookup func(string) (string, bool)) int {
	fs := flag.NewFlagSet("self-update", flag.ContinueOnError)
	fs.SetOutput(errOut)
	check := fs.Bool("check", false, "only report the target version")
	apply := fs.Bool("apply", false, "install the target and restart now")
	allowPkg := fs.Bool("allow-package-managed", false, "update in place even though a package manager owns the binary")
	force := fs.String("force-target", "", "install this release tag instead of the control plane answer (needs --apply)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *check && *apply {
		_, _ = fmt.Fprintln(errOut, "pass only one of --check and --apply")
		return 2
	}
	if *force != "" && !*apply {
		_, _ = fmt.Fprintln(errOut, "--force-target needs --apply: "+agentupdate.ForceCommand(*force))
		return 2
	}
	if *allowPkg {
		_, _ = fmt.Fprintln(errOut, "note: updating a package-managed binary in place; the package manager may replace it later. The download is still signature verified and rolls back on failure.")
	}
	o, err := buildUpdateOptions(lookup, *allowPkg)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		if errors.Is(err, agentupdate.ErrPackageManaged) {
			_, _ = fmt.Fprintln(errOut, "next step: "+agentupdate.PackageManagedHelp)
			return exitPackageManaged
		}
		return 1
	}
	o.Check, o.Apply, o.ForceTarget = *check, *apply, *force
	ctx, cancel := context.WithTimeout(context.Background(), updateHTTPTimeout)
	defer cancel()
	res, err := agentupdate.Run(ctx, o)
	if b, jerr := json.Marshal(res); jerr == nil {
		_, _ = fmt.Fprintln(out, string(b))
	}
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return 1
	}
	return 0
}

// autoUpdateLoop checks on an interval and installs only when the control
// plane setting nodes.agent_auto_update is on.
func autoUpdateLoop(ctx context.Context, lookup func(string) (string, bool), logger *slog.Logger) {
	if !updateConfigured(lookup) {
		return
	}
	every := time.Hour
	if v, ok := lookup(envUpdateInterval); ok {
		if n, err := strconv.Atoi(v); err == nil && n >= 60 {
			every = time.Duration(n) * time.Second
		}
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		o, err := buildUpdateOptions(lookup, false)
		if errors.Is(err, agentupdate.ErrPackageManaged) {
			logger.Info("agent self-update off: package managed; the agent keeps running", slog.String("next_step", agentupdate.PackageManagedHelp))
			return
		}
		if err != nil {
			logger.Warn("agent update check skipped", slog.String("error", err.Error()))
			continue
		}
		res, err := agentupdate.Run(ctx, o)
		if err != nil {
			// The running agent is untouched; the loop just tries again next interval.
			logger.Warn("agent update did not apply; the agent keeps running", slog.String("error", err.Error()))
			continue
		}
		logger.Info(updateStatusLine(res))
	}
}

// updateStatusLine is the one-line update status: checked, available, held
// with its reason, or installed.
func updateStatusLine(res agentupdate.Result) string {
	switch res.Action {
	case agentupdate.ActionHold:
		return fmt.Sprintf("agent update held: %s (target %s, reason: %s)", res.Current, res.Target, res.Reason)
	case agentupdate.ActionAvailable:
		return fmt.Sprintf("agent update available: %s -> %s", res.Current, res.Target)
	case agentupdate.ActionStaged, agentupdate.ActionRestarted:
		return fmt.Sprintf("agent update installed: %s -> %s", res.Current, res.Target)
	case agentupdate.ActionRolled:
		return fmt.Sprintf("agent update rolled back to %s", res.Current)
	}
	if res.Target != "" {
		return fmt.Sprintf("agent update waiting: target %s (reason: %s)", res.Target, res.Reason)
	}
	return "agent update checked: up to date at " + res.Current
}

// rollbackToPrev swaps the kept .prev binary back after the health tracker
// reports failure, so a bad release does not keep crash looping.
func rollbackToPrev(ctx context.Context, lookup func(string) (string, bool), logger *slog.Logger, dataDir string) {
	exe, err := cliupdate.ExecutablePath()
	if err != nil {
		return
	}
	if _, err := os.Stat(agentupdate.PrevPath(exe)); err != nil {
		return
	}
	if _, err := agentupdate.Rollback(ctx, exe, dataDir, systemService(lookup)); err != nil {
		logger.Error("agent rollback failed", slog.String("error", err.Error()))
		return
	}
	logger.Warn("rolled the agent back to the previous binary")
}
