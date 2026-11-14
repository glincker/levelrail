package cloudiac

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

// Source: https://opentofu.org/docs/intro/install/standalone/ "Verifying the binaries with Cosign".
const (
	tofuSigningIssuer   = "https://token.actions.githubusercontent.com"
	tofuSigningWorkflow = "https://github.com/opentofu/opentofu/.github/workflows/release.yml@refs/heads/v"
)

var stableVersion = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)$`)

// ErrChecksumNotListed means the signed checksum file has no line for the downloaded archive.
var ErrChecksumNotListed = errors.New("cloudiac: archive is not listed in the signed checksum file")

// ReleaseMaterial is the checksum file of one release plus its Sigstore bundle.
// A bare .sig and .pem pair carries no transparency log proof, so a bundle is required.
type ReleaseMaterial struct {
	Sums   []byte
	Entity verify.SignedEntity
}

// MaterialSource loads the release material for a version, for example from a fetched bundle.
type MaterialSource func(ctx context.Context, version string) (ReleaseMaterial, error)

// ReleaseVerifier checks the keyless signature of the OpenTofu checksum file and then the archive against it.
type ReleaseVerifier struct {
	version  string
	trusted  root.TrustedMaterial
	material MaterialSource
	// ctThreshold is 1 in production; virtual test certificates carry no SCT.
	ctThreshold int
}

// NewReleaseVerifier binds the verifier to the pinned version, whose minor branch fixes the signing identity.
func NewReleaseVerifier(version string, trusted root.TrustedMaterial, material MaterialSource) *ReleaseVerifier {
	return &ReleaseVerifier{version: version, trusted: trusted, material: material, ctThreshold: 1}
}

// LoadSigstoreTrustedRoot fetches the public Sigstore trust root over TUF, caching it under cacheDir.
func LoadSigstoreTrustedRoot(cacheDir string) (*root.TrustedRoot, error) {
	opts := tuf.DefaultOptions()
	if cacheDir != "" {
		opts.CachePath = cacheDir
	}
	return root.FetchTrustedRootWithOptions(opts)
}

// ParseReleaseBundle pairs the checksum file with a Sigstore bundle in its JSON form.
func ParseReleaseBundle(sums, bundleJSON []byte) (ReleaseMaterial, error) {
	var b bundle.Bundle
	if err := b.UnmarshalJSON(bundleJSON); err != nil {
		return ReleaseMaterial{}, fmt.Errorf("%w: %w", ErrSignatureUnverified, err)
	}
	return ReleaseMaterial{Sums: sums, Entity: &b}, nil
}

// signingIdentity refuses pre-releases: those are signed from main, which the pinned path never uses.
func signingIdentity(version string) (string, error) {
	m := stableVersion.FindStringSubmatch(version)
	if m == nil {
		return "", fmt.Errorf("%w: %q is not a stable release", ErrSignatureUnverified, version)
	}
	return tofuSigningWorkflow + m[1] + "." + m[2], nil
}

// Verify implements SignatureVerifier. Anything missing fails closed with ErrSignatureUnverified.
func (v *ReleaseVerifier) Verify(ctx context.Context, archiveName, archivePath string) error {
	if v == nil || v.trusted == nil || v.material == nil {
		return ErrSignatureUnverified
	}
	identity, err := signingIdentity(v.version)
	if err != nil {
		return err
	}
	mat, err := v.material(ctx, v.version)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSignatureUnverified, err)
	}
	if mat.Entity == nil || len(mat.Sums) == 0 {
		return ErrSignatureUnverified
	}
	if err := v.verifySums(mat, identity); err != nil {
		return err
	}
	want, err := listedDigest(mat.Sums, archiveName)
	if err != nil {
		return err
	}
	got, err := fileSHA256(archivePath)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		return ErrChecksumMismatch
	}
	return nil
}

func (v *ReleaseVerifier) verifySums(mat ReleaseMaterial, identity string) error {
	opts := []verify.VerifierOption{verify.WithTransparencyLog(1), verify.WithObserverTimestamps(1)}
	if v.ctThreshold > 0 {
		opts = append(opts, verify.WithSignedCertificateTimestamps(v.ctThreshold))
	}
	sv, err := verify.NewVerifier(v.trusted, opts...)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSignatureUnverified, err)
	}
	id, err := verify.NewShortCertificateIdentity(tofuSigningIssuer, "", identity, "")
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSignatureUnverified, err)
	}
	policy := verify.NewPolicy(verify.WithArtifact(bytes.NewReader(mat.Sums)), verify.WithCertificateIdentity(id))
	if _, err := sv.Verify(mat.Entity, policy); err != nil {
		return fmt.Errorf("%w: %w", ErrSignatureUnverified, err)
	}
	return nil
}

// listedDigest reads "<sha256>  <name>" lines and refuses a file that lists the name twice.
func listedDigest(sums []byte, name string) (string, error) {
	var found string
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		if found != "" {
			return "", fmt.Errorf("%w: %s listed twice", ErrChecksumNotListed, name)
		}
		if raw, err := hex.DecodeString(fields[0]); err != nil || len(raw) != sha256.Size {
			return "", fmt.Errorf("%w: malformed digest for %s", ErrChecksumNotListed, name)
		}
		found = strings.ToLower(fields[0])
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("%w: %s", ErrChecksumNotListed, name)
	}
	return found, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // path is the fetcher's own temp file
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
