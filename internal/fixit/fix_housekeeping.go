package fixit

import (
	"context"
	"fmt"
)

// OrphansDeps are the facts and the pruner the orphaned-Docker-data fix uses.
type OrphansDeps struct {
	Check       func(ctx context.Context) (CheckState, error)
	Reclaimable func(ctx context.Context) (DiskReclaim, error)
	Prune       func(ctx context.Context) (freed int64, err error)
}

// OrphansFix previews and prunes unreferenced Docker data for the docker_orphans check.
func OrphansFix(d OrphansDeps) Fix {
	return Fix{
		ID: "docker.orphans.prune", CheckID: "docker_orphans", Subject: "docker", Kind: KindAuto,
		Title: "Remove orphaned Docker data",
		Why:   "Unreferenced containers, images, volumes, build cache and networks use disk and slow builds. Rollback targets and volumes in use are kept.",
		Risk:  RiskSafe, Reversible: false, Needs: "root",
		Detect: func(ctx context.Context) (Applicability, error) {
			c, err := d.Check(ctx)
			if err != nil {
				return Applicability{}, err
			}
			a := Applicability{Failing: c.Failing, Detail: c.Detail, Signature: "orphans", Changes: []Change{}}
			if !c.Failing {
				return a, nil
			}
			r, err := d.Reclaimable(ctx)
			if err != nil {
				return Applicability{}, err
			}
			add := func(label string, b int64) {
				if b > 0 {
					a.Changes = append(a.Changes, Change{Label: label, Bytes: b})
				}
			}
			add("Stopped containers not managed by an app", r.ContainersBytes)
			add("Dangling images (rollback targets are kept)", r.ImagesBytes)
			add("Anonymous volumes", r.VolumesBytes)
			add("Build cache", r.BuildCacheBytes)
			if n := len(r.OrphanNetworks); n > 0 {
				a.Changes = append(a.Changes, Change{Label: "Orphaned networks", Detail: fmt.Sprintf("%d network(s): %v", n, r.OrphanNetworks)})
			}
			return a, nil
		},
		Run: func(ctx context.Context, _ Input) error {
			if d.Prune == nil {
				return ErrCannotFixHere
			}
			if _, err := d.Prune(ctx); err != nil {
				return fmt.Errorf("cleanup failed: %w", err)
			}
			return nil
		},
	}
}

// LegacyTokensDeps are the facts and the importer the legacy-token fix uses.
type LegacyTokensDeps struct {
	// Pending counts pre-upgrade tokens not yet migrated.
	Pending func(ctx context.Context) (int, error)
	// Migrate imports every pending token; nil means the auth library is not configured.
	Migrate func(ctx context.Context) error
}

// LegacyTokensFix migrates pending pre-upgrade API tokens now instead of on first use.
func LegacyTokensFix(d LegacyTokensDeps) Fix {
	return Fix{
		ID: "tokens.legacy.migrate", CheckID: "legacy_tokens", Subject: "security", Kind: KindAuto,
		Title: "Migrate pre-upgrade API tokens",
		Why:   "Tokens minted before the auth library only migrate on next start or first use. Migrating now keeps the same bearer strings working.",
		Risk:  RiskSafe, Reversible: false, Needs: "root",
		Detect: func(ctx context.Context) (Applicability, error) {
			n, err := d.Pending(ctx)
			if err != nil {
				return Applicability{}, err
			}
			a := Applicability{Failing: n > 0, Signature: "pending", Changes: []Change{}}
			if n == 0 {
				a.Detail = "no tokens waiting to migrate"
				return a, nil
			}
			a.Detail = fmt.Sprintf("%d token(s) not yet migrated", n)
			a.Changes = []Change{{Label: "Import pending tokens into the auth library", Detail: fmt.Sprintf("%d token(s); expired or revoked ones are skipped", n)}}
			return a, nil
		},
		Run: func(ctx context.Context, _ Input) error {
			if d.Migrate == nil {
				return ErrCannotFixHere
			}
			if err := d.Migrate(ctx); err != nil {
				return fmt.Errorf("token migration failed: %w", err)
			}
			return nil
		},
	}
}
