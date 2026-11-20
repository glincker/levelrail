package buildrecord

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
	if c.Digest != digest || c.App != app || c.BuilderTrust != tier {
		return errors.New("build record: record does not match the built image")
	}
	return g.Store.LinkAttempt(ctx, recordID, attemptID)
}

// link is best effort for tiers that do not depend on the record.
func (g *Gate) link(ctx context.Context, recordID, attemptID string) {
	if recordID != "" && attemptID != "" && g.Store != nil {
		_ = g.Store.LinkAttempt(ctx, recordID, attemptID)
	}
}
