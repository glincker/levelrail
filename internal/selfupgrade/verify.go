package selfupgrade

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Refusals that change nothing on the host.
var (
	ErrChecksumMismatch  = errors.New("downloaded binary does not match its published checksum")
	ErrChecksumMissing   = errors.New("release publishes no checksum for this binary")
	ErrSignatureInvalid  = errors.New("release signature verification failed")
	ErrSignatureRequired = errors.New("signature verification is required but unavailable")
	ErrBreakingNotAcked  = errors.New("breaking changes need acknowledgement")
	ErrDowngrade         = errors.New("target release is older than the database schema")
	ErrVersionMismatch   = errors.New("downloaded binary reports a different version than requested")
	ErrMigrationCheck    = errors.New("migration dry-run failed")
	ErrNotHealthy        = errors.New("new release did not become healthy")
	// ErrDryRunUnsupported is returned by a migration check when the target
	// binary has no dry-run subcommand, which is not a failure.
	ErrDryRunUnsupported = errors.New("target binary has no migration dry-run")
)

// ParseChecksums reads sha256sum output into name to hex digest.
func ParseChecksums(r io.Reader) (map[string]string, error) {
	out := map[string]string{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 || len(fields[0]) != sha256.Size*2 {
			continue
		}
		out[strings.TrimPrefix(fields[1], "*")] = strings.ToLower(fields[0])
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read checksums: %w", err)
	}
	return out, nil
}

// VerifyChecksum checks the file at path against asset's entry in the
// checksums file.
func VerifyChecksum(path, checksumsPath, asset string) error {
	cf, err := os.Open(checksumsPath) //nolint:gosec // path is under the upgrade's own work directory
	if err != nil {
		return fmt.Errorf("open checksums: %w", err)
	}
	defer func() { _ = cf.Close() }()
	sums, err := ParseChecksums(cf)
	if err != nil {
		return err
	}
	want, ok := sums[asset]
	if !ok {
		return fmt.Errorf("%w: %s is not listed in checksums.txt", ErrChecksumMissing, asset)
	}
	f, err := os.Open(path) //nolint:gosec // path is under the upgrade's own work directory
	if err != nil {
		return fmt.Errorf("open binary: %w", err)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash binary: %w", err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, want, got)
	}
	return nil
}

// Signature modes, the same vocabulary as the installer's APP_INSTALL_VERIFY.
const (
	VerifyAuto    = "auto"
	VerifyRequire = "require"
	VerifyOff     = "off"
)

// Signer verifies a cosign keyless bundle over a checksums file.
type Signer struct {
	Mode           string
	CosignPath     string
	IdentityRegexp string
	Issuer         string
	// Run executes the verifier; nil uses os/exec. Tests inject a fake.
	Run func(ctx context.Context, name string, args ...string) ([]byte, error)
}

// Verify returns a human detail for the timeline. A failed verification is
// always an error. A missing cosign or bundle is an error only in require mode.
func (s Signer) Verify(ctx context.Context, checksumsPath, bundlePath string) (string, error) {
	if s.Mode == VerifyOff {
		return "signature check disabled", nil
	}
	if bundlePath == "" {
		if s.Mode == VerifyRequire {
			return "", fmt.Errorf("%w: release publishes no signature bundle", ErrSignatureRequired)
		}
		return "release publishes no signature, checksum only", nil
	}
	bin := s.CosignPath
	if bin == "" {
		bin = "cosign"
	}
	run := s.Run
	if run == nil {
		if _, err := exec.LookPath(bin); err != nil {
			if s.Mode == VerifyRequire {
				return "", fmt.Errorf("%w: cosign is not installed", ErrSignatureRequired)
			}
			return "cosign not installed, checksum only", nil
		}
		run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).CombinedOutput() //nolint:gosec // fixed verifier binary, argv only
		}
	}
	out, err := run(ctx, bin, "verify-blob", "--bundle", bundlePath,
		"--certificate-identity-regexp", s.IdentityRegexp,
		"--certificate-oidc-issuer", s.Issuer, checksumsPath)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrSignatureInvalid, strings.TrimSpace(string(out)))
	}
	return "cosign keyless signature verified", nil
}
