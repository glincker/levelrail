package selfupgrade

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

// errTrustRoot marks a failure to load the Sigstore trust root, which is an
// availability problem rather than a bad signature.
var errTrustRoot = errors.New("sigstore trust root unavailable")

// verifyBundle checks a keyless bundle over the artifact at artifactPath
// without any external binary. cacheDir keeps the TUF metadata between runs.
func verifyBundle(artifactPath, bundlePath, identityRegexp, issuer, cacheDir string) error {
	b, err := bundle.LoadJSONFromPath(bundlePath)
	if err != nil {
		return fmt.Errorf("read signature bundle: %w", err)
	}
	tr, err := loadTrustedRoot(cacheDir)
	if err != nil {
		return fmt.Errorf("%w: %w", errTrustRoot, err)
	}
	f, err := os.Open(artifactPath) //nolint:gosec // path is under the upgrade's own work directory
	if err != nil {
		return fmt.Errorf("open checksums: %w", err)
	}
	defer func() { _ = f.Close() }()
	return verifyEntity(b, tr, f, identityRegexp, issuer, verify.WithSignedCertificateTimestamps(1))
}

func loadTrustedRoot(cacheDir string) (*root.TrustedRoot, error) {
	opts := tuf.DefaultOptions()
	if cacheDir != "" {
		opts.CachePath = cacheDir
	}
	return root.FetchTrustedRootWithOptions(opts)
}

func verifyEntity(entity verify.SignedEntity, tm root.TrustedMaterial, artifact io.Reader, identityRegexp, issuer string, extra ...verify.VerifierOption) error {
	v, err := verify.NewVerifier(tm, append([]verify.VerifierOption{
		verify.WithTransparencyLog(1),
		verify.WithObserverTimestamps(1),
	}, extra...)...)
	if err != nil {
		return fmt.Errorf("build verifier: %w", err)
	}
	id, err := verify.NewShortCertificateIdentity(issuer, "", "", identityRegexp)
	if err != nil {
		return fmt.Errorf("certificate identity: %w", err)
	}
	if _, err := v.Verify(entity, verify.NewPolicy(verify.WithArtifact(artifact), verify.WithCertificateIdentity(id))); err != nil {
		return err
	}
	return nil
}
