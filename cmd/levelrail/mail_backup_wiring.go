package main

import (
	"filippo.io/age"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/secrets"
)

// mailBackupOption reuses the instance backup encryption key, so mail
// backups need no key storage of their own.
func mailBackupOption(r *backup.Runner, mgr *secrets.Manager) api.Option {
	var recipients []age.Recipient
	if r != nil && r.Volume != nil && r.Volume.Sealer != nil {
		recipients = r.Volume.Sealer.Recipients
	}
	if mgr == nil {
		return api.WithMailBackup(backupDataDir(), recipients, backupIdentities(r), nil)
	}
	return api.WithMailBackup(backupDataDir(), recipients, backupIdentities(r), mgr)
}
