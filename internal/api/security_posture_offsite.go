package api

import "context"

// collectPostureOffsite lists offsite backup destinations that store dumps unencrypted.
func (rt *Router) collectPostureOffsite(ctx context.Context, in *postureInput) {
	if rt.backupTargets == nil {
		return
	}
	targets, err := rt.backupTargets.ListBackupTargets(ctx)
	if err != nil {
		return
	}
	for _, t := range targets {
		if !t.EncryptDumps {
			in.PlainOffsite = append(in.PlainOffsite, t.Name)
		}
	}
	in.OffsiteKnown = true
}

// offsiteEncryptionRule warns when any offsite destination keeps dumps in plain form.
func offsiteEncryptionRule(in postureInput) postureItem {
	const id = "offsite_backup_unencrypted"
	switch {
	case !in.OffsiteKnown:
		return unknown(id, postureMedium, "backup destinations could not be read")
	case len(in.PlainOffsite) == 0:
		return pass(id, postureMedium)
	}
	return failing(id, postureMedium, in.PlainOffsite, linkFix("/settings/backup-targets"))
}
