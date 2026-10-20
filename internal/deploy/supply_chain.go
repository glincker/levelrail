package deploy

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/build"
)

// SupplyChainHook stores a build's SBOM and may veto the release. A non-nil
// error stops the deploy before desired state changes, so the previous release
// keeps serving. *supplychain.Service satisfies it.
type SupplyChainHook interface {
	AfterBuild(ctx context.Context, app, attemptID string, att build.Attestations) error
}

// WithSupplyChain enables SBOM storage and the scan gate for built images.
func WithSupplyChain(h SupplyChainHook) Option {
	return func(p *Pipeline) { p.supplyChain = h }
}

// WithBuildAttest asks BuildKit for an SBOM and minimal provenance attestation
// on Dockerfile builds.
func WithBuildAttest(on bool) Option {
	return func(p *Pipeline) { p.attest = on }
}
