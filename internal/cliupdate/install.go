package cliupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// EnvInstallMethod is set by the npm launcher so the binary knows a package manager owns it.
const EnvInstallMethod = "LEVELRAIL_CLI_INSTALL_METHOD"

// Install methods. Binary means a plain download the CLI may replace itself.
const (
	MethodBinary = "binary"
	MethodNPM    = "npm"
	MethodPNPM   = "pnpm"
	MethodYarn   = "yarn"
	MethodNPX    = "npx"
	MethodBrew   = "brew"
)

// DetectMethod picks the install method from the launcher env var, then from
// where the executable lives. Unknown input falls back to a plain binary.
func DetectMethod(exePath string, lookupEnv func(string) (string, bool)) string {
	p := filepath.ToSlash(exePath)
	fromPath := methodFromPath(p)
	if lookupEnv != nil {
		if v, ok := lookupEnv(EnvInstallMethod); ok {
			switch m := strings.ToLower(strings.TrimSpace(v)); m {
			case MethodNPM:
				// The launcher only says npm; the path tells pnpm, yarn and npx apart.
				if fromPath == MethodPNPM || fromPath == MethodYarn || fromPath == MethodNPX {
					return fromPath
				}
				return MethodNPM
			case MethodPNPM, MethodYarn, MethodNPX, MethodBrew, MethodBinary:
				return m
			}
		}
	}
	return fromPath
}

func methodFromPath(p string) string {
	switch {
	case strings.Contains(p, "/Cellar/") || strings.HasPrefix(p, "/opt/homebrew/") || strings.HasPrefix(p, "/home/linuxbrew/"):
		return MethodBrew
	case strings.Contains(p, "/_npx/"):
		return MethodNPX
	case strings.Contains(p, "/node_modules/") && (strings.Contains(p, "/.pnpm/") || strings.Contains(p, "/pnpm/")):
		return MethodPNPM
	case strings.Contains(p, "/node_modules/") && (strings.Contains(p, "/.yarn/") || strings.Contains(p, "/yarn/")):
		return MethodYarn
	case strings.Contains(p, "/node_modules/"):
		return MethodNPM
	}
	return MethodBinary
}

// PackageManaged reports whether a JS package manager owns the install.
func PackageManaged(method string) bool {
	switch method {
	case MethodNPM, MethodPNPM, MethodYarn, MethodNPX:
		return true
	}
	return false
}

// UpgradeArgv returns the package manager invocation that upgrades the CLI
// to spec ("latest" or a version), or nil when the method has no such command.
func UpgradeArgv(method, spec string) []string {
	pkg := "levelrail-cli@" + spec
	switch method {
	case MethodNPM:
		return []string{"npm", "install", "--global", pkg}
	case MethodPNPM:
		return []string{"pnpm", "add", "--global", pkg}
	case MethodYarn:
		return []string{"yarn", "global", "add", pkg}
	}
	return nil
}

// UpgradeCommand returns the right upgrade command for the method, or the
// CLI's own update command for a plain binary.
func UpgradeCommand(method, prog string) string {
	switch method {
	case MethodNPX:
		return "npx levelrail-cli@latest (npx fetches the latest on each run)"
	case MethodBrew:
		return "brew upgrade levelrail-cli"
	}
	if argv := UpgradeArgv(method, "latest"); argv != nil {
		return strings.Join(argv, " ")
	}
	return prog + " update"
}

// RunUpgrade runs the package manager upgrade with the user's terminal attached.
func RunUpgrade(ctx context.Context, argv []string, stdout, stderr io.Writer) error {
	if len(argv) == 0 {
		return fmt.Errorf("no upgrade command for this install")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // argv comes from UpgradeArgv with a validated tag
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", strings.Join(argv, " "), err)
	}
	return nil
}

// InstalledVersion asks the executable at path for its version after an upgrade.
func InstalledVersion(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, smokeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "version", "--json") //nolint:gosec // path is this CLI's own executable
	cmd.Env = []string{"LEVELRAIL_CI=1"}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	var report struct {
		CLI struct {
			Version string `json:"version"`
		} `json:"cli"`
	}
	if err := json.Unmarshal(out, &report); err != nil {
		return "", err
	}
	return report.CLI.Version, nil
}

// MinorBehind reports whether server is at least one minor (or major) ahead of cli.
func MinorBehind(cli, server string) bool {
	cm, ci, ok1 := majorMinor(cli)
	sm, si, ok2 := majorMinor(server)
	if !ok1 || !ok2 {
		return false
	}
	return sm > cm || (sm == cm && si > ci)
}

func majorMinor(v string) (major, minor int, ok bool) {
	parts := strings.SplitN(strings.TrimPrefix(strings.TrimSpace(v), "v"), ".", 3)
	if len(parts) < 2 {
		return 0, 0, false
	}
	a, err1 := strconv.Atoi(parts[0])
	b, err2 := strconv.Atoi(parts[1])
	return a, b, err1 == nil && err2 == nil
}
