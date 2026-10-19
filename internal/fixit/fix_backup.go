package fixit

import (
	"context"
	"fmt"
)

// BackupDeps are the facts and the action the stale-backup fix uses.
type BackupDeps struct {
	Check func(ctx context.Context) (CheckState, error)
	// Create takes a snapshot and returns its name; nil means backups are not configured.
	Create func(ctx context.Context) (string, error)
}

// ControlPlaneBackupFix takes a fresh control plane snapshot when the newest one is stale.
func ControlPlaneBackupFix(d BackupDeps) Fix {
	return Fix{
		ID: "backup.control_plane.snapshot", CheckID: "control_plane_backup", Subject: "backup", Kind: KindAuto,
		Title: "Take a control plane snapshot now",
		Why:   "The newest snapshot is old, so a lost disk would lose recent changes.",
		Risk:  RiskSafe, Reversible: true, Needs: "root",
		Detect: func(ctx context.Context) (Applicability, error) {
			c, err := d.Check(ctx)
			if err != nil {
				return Applicability{}, err
			}
			a := Applicability{Failing: c.Failing, Detail: c.Detail, Signature: "stale", Changes: []Change{}}
			if c.Failing {
				a.Changes = []Change{{Label: "Write a new control plane snapshot", Detail: "older snapshots are kept"}}
			}
			return a, nil
		},
		Run: func(ctx context.Context, _ Input) error {
			if d.Create == nil {
				return ErrCannotFixHere
			}
			if _, err := d.Create(ctx); err != nil {
				return fmt.Errorf("snapshot failed: %w", err)
			}
			return nil
		},
	}
}
