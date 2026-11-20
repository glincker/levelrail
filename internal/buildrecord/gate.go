package buildrecord

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Builder tiers, mirroring the node trust tiers.
const (
	TierServer   = "server"
	TierManaged  = "managed"
	TierPersonal = "personal"
)

// Policy is what an environment says about who may build for it.
type Policy struct {
	EnvKind string
	// EnvName is the environment the claim's environment is compared with, when known.
	EnvName string
	// Allowed is the explicit allow-list of tiers or node IDs; it decides alone when set.
	Allowed        []string
	PersonalInProd bool
}

// TierAllowed reports whether a builder of this tier or node may produce images for the environment.
func (p Policy) TierAllowed(tier, nodeID string) bool {
	if tier == "" {
		tier = TierServer
	}
	if len(p.Allowed) > 0 {
		return slices.Contains(p.Allowed, tier) || (nodeID != "" && slices.Contains(p.Allowed, nodeID))
	}
	return tier != TierPersonal || p.EnvKind != "production" || p.PersonalInProd
}

// Store is where signed records live.
type Store interface {
	SaveRecord(ctx context.Context, id, app string, s Signed) error
	GetRecord(ctx context.Context, id string) (Signed, error)
	LinkAttempt(ctx context.Context, id, attemptID string) error
}

// ErrNoRecord means no signed record exists for the image.
var ErrNoRecord = errors.New("build record: none for this image")

// ImageStore finds the record of an image about to be deployed.
type ImageStore interface {
	RecordByDigest(ctx context.Context, digest string) (string, Signed, error)
	RecordByRepo(ctx context.Context, repo string) (string, Signed, error)
}

// Gate refuses to deploy an image whose builder the environment does not allow
// or whose personal-tier record does not verify.
type Gate struct {
	Signer *Signer
	Store  Store
	Policy func(ctx context.Context, app string) (Policy, error)
}

// CheckBuild fails closed. Server tier images and the tar path carry no record and pass.
func (g *Gate) CheckBuild(ctx context.Context, app, attemptID, recordID, digest, tier, nodeID string) error {
	if tier == "" || tier == TierServer {
		return nil
	}
	if g == nil || g.Policy == nil {
		return errors.New("build record: no policy available to check a remote builder")
	}
	pol, err := g.Policy(ctx, app)
	if err != nil {
		return fmt.Errorf("build record: load environment policy: %w", err)
	}
	if !pol.TierAllowed(tier, nodeID) {
		return fmt.Errorf("build record: %s builders may not build for this environment", tier)
	}
	if tier != TierPersonal {
		g.link(ctx, recordID, attemptID)
		return nil
	}
	if recordID == "" || g.Signer == nil || g.Store == nil {
		return errors.New("build record: personal builder result has no signed record")
	}
	rec, err := g.Store.GetRecord(ctx, recordID)
	if err != nil {
		return fmt.Errorf("build record: load record: %w", err)
	}
	if err := g.Signer.Verify(rec); err != nil {
		return err
	}
	c := rec.Claims
	if c.Digest != digest || c.App != app || c.BuilderTrust != tier || (nodeID != "" && c.BuilderNodeID != nodeID) {
		return errors.New("build record: record does not match the built image")
	}
	if c.Environment != "" && pol.EnvName != "" && c.Environment != pol.EnvName {
		return errors.New("build record: record was issued for another environment")
	}
	return g.Store.LinkAttempt(ctx, recordID, attemptID)
}

// CheckImage vets an image about to become app's desired image, whatever path
// brought it there. An image with no record was not built remotely and passes;
// a lookup or policy failure refuses.
func (g *Gate) CheckImage(ctx context.Context, app, image string) error {
	if g == nil || g.Policy == nil {
		return errors.New("build record: no policy available to check this image")
	}
	is, ok := g.Store.(ImageStore)
	if !ok {
		return errors.New("build record: record lookup is unavailable")
	}
	digest := ""
	if i := strings.LastIndex(image, "@"); i > 0 {
		digest = image[i+1:]
	}
	var rec Signed
	var err error
	if digest != "" {
		_, rec, err = is.RecordByDigest(ctx, digest)
	} else {
		_, rec, err = is.RecordByRepo(ctx, image)
	}
	if errors.Is(err, ErrNoRecord) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("build record: look up image record: %w", err)
	}
	pol, err := g.Policy(ctx, app)
	if err != nil {
		return fmt.Errorf("build record: load environment policy: %w", err)
	}
	c := rec.Claims
	if !pol.TierAllowed(c.BuilderTrust, c.BuilderNodeID) {
		return fmt.Errorf("build record: %s builders may not build for this environment", c.BuilderTrust)
	}
	if c.BuilderTrust != TierPersonal {
		return nil
	}
	if g.Signer == nil {
		return errors.New("build record: cannot verify a personal builder record")
	}
	if err := g.Signer.Verify(rec); err != nil {
		return err
	}
	if digest != "" && c.Digest != digest {
		return errors.New("build record: record does not match the image")
	}
	return nil
}

// link is best effort for tiers that do not depend on the record.
func (g *Gate) link(ctx context.Context, recordID, attemptID string) {
	if recordID != "" && attemptID != "" && g.Store != nil {
		_ = g.Store.LinkAttempt(ctx, recordID, attemptID)
	}
}
