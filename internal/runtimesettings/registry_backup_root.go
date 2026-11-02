package runtimesettings

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Backup root key.
const (
	KeyBackupLocalRoot = "backup.local_root"
)

// backupRootCheck allows empty (use the data dir default) or a clean absolute path with no dot-dot segment.
func backupRootCheck(canonical string) error {
	if canonical == "" {
		return nil
	}
	for _, seg := range strings.Split(filepath.ToSlash(canonical), "/") {
		if seg == ".." {
			return errors.New("path must not contain a parent directory segment")
		}
	}
	if !filepath.IsAbs(canonical) || filepath.Clean(canonical) == "/" {
		return fmt.Errorf("path must be an absolute directory other than /")
	}
	return nil
}

func init() {
	Register(Definition{
		Key: KeyBackupLocalRoot, Category: CategoryDeploys, Kind: KindString, Default: "",
		Env: "APP_BACKUP_LOCAL_ROOT", Apply: ApplyLive, CheckValue: backupRootCheck,
		TitleKey: "runtimeSettings:items.backupLocalRoot.title", DescKey: "runtimeSettings:items.backupLocalRoot.description",
		Title:       "Local backup root",
		Description: "Absolute directory on the control plane that confines local backup targets. Empty uses backups/ under the data directory. The next backup or restore uses a changed value; target paths stay relative to it and cannot leave it.",
	})
}
