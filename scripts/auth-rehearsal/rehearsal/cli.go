package rehearsal

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// CLIResult is one run of the real backfill command.
type CLIResult struct {
	Output  string
	Wall    time.Duration
	PeakRSS int64
	Err     error
}

// BuildBinary compiles the control plane from repoRoot into dir and returns its path.
func BuildBinary(ctx context.Context, repoRoot, dir string) (string, error) {
	bin := filepath.Join(dir, "rehearsal-bin")
	cmd := exec.CommandContext(ctx, "go", "build", "-o", bin, "./cmd/levelrail") //nolint:gosec // fixed arguments
	cmd.Dir = repoRoot
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("rehearsal: build control plane: %w\n%s", err, out.String())
	}
	return bin, nil
}

// RunBackfill runs `<bin> auth-backfill [--dry-run]` against dataDir and
// measures wall time and the child's peak resident set size.
func RunBackfill(ctx context.Context, bin, dataDir string, dryRun bool) CLIResult {
	args := []string{"auth-backfill"}
	if dryRun {
		args = append(args, "--dry-run")
	}
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // harness-built binary
	env := make([]string, 0, len(os.Environ())+1)
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "APP_MASTER_KEY=") && !strings.HasPrefix(e, "APP_DATA_DIR=") {
			env = append(env, e)
		}
	}
	cmd.Env = append(env, "APP_DATA_DIR="+dataDir)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	start := time.Now()
	err := cmd.Run()
	res := CLIResult{Output: out.String(), Wall: time.Since(start), Err: err}
	if cmd.ProcessState != nil {
		if ru, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage); ok {
			res.PeakRSS = ru.Maxrss
			if runtime.GOOS != "darwin" {
				res.PeakRSS *= 1024
			}
		}
	}
	return res
}

// CLIReport is the counts the backfill command printed. OAuth is -1 when the
// command does not report an OAuth identity count.
type CLIReport struct {
	Users, AlreadyMapped, Passwords, Tokens, TokensSkipped int
	Passkeys, TOTP, RecoveryNotMoved, OAuth                int
}

var (
	reLine     = regexp.MustCompile(`(?m)^\s+(users copied|password hashes copied|api tokens copied|passkeys copied|totp secrets copied):\s+(\d+)`)
	reMapped   = regexp.MustCompile(`already mapped: (\d+)`)
	reSkipped  = regexp.MustCompile(`skipped, no owner: (\d+)`)
	reRecovery = regexp.MustCompile(`recovery codes are not converted; (\d+) user`)
	reOAuth    = regexp.MustCompile(`(?mi)^\s+[^\n]*oauth[^\n:]*:\s+(\d+)`)
)

// ParseReport reads the counts out of the backfill command's output.
func ParseReport(out string) CLIReport {
	r := CLIReport{OAuth: -1}
	for _, m := range reLine.FindAllStringSubmatch(out, -1) {
		n, _ := strconv.Atoi(m[2])
		switch m[1] {
		case "users copied":
			r.Users = n
		case "password hashes copied":
			r.Passwords = n
		case "api tokens copied":
			r.Tokens = n
		case "passkeys copied":
			r.Passkeys = n
		case "totp secrets copied":
			r.TOTP = n
		}
	}
	r.AlreadyMapped = firstInt(reMapped, out)
	r.TokensSkipped = firstInt(reSkipped, out)
	r.RecoveryNotMoved = firstInt(reRecovery, out)
	if m := reOAuth.FindStringSubmatch(out); m != nil {
		r.OAuth, _ = strconv.Atoi(m[1])
	}
	return r
}

func firstInt(re *regexp.Regexp, s string) int {
	m := re.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

func readFile(p string) ([]byte, error) {
	b, err := os.ReadFile(p) //nolint:gosec // operator-supplied harness path
	if err != nil {
		return nil, fmt.Errorf("rehearsal: read %s: %w", p, err)
	}
	return b, nil
}
