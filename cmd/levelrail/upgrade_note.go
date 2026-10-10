package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/GLINCKER/levelrail/internal/upgradehistory"
	"github.com/GLINCKER/levelrail/internal/version"
)

const (
	upgradeNoteUsage     = "usage: levelrail upgrade-note --by NAME [--method manual|package|ci] [--reason TEXT] [--data-dir DIR]"
	upgradeNoteMaxByLen  = 64
	upgradeNoteMaxReason = 120
)

type upgradeNoteFlags struct {
	by, method, reason, dataDir string
}

func parseUpgradeNoteFlags(args []string) (upgradeNoteFlags, error) {
	var f upgradeNoteFlags
	fs := flag.NewFlagSet("upgrade-note", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&f.by, "by", "", "")
	fs.StringVar(&f.method, "method", upgradehistory.MethodManual, "")
	fs.StringVar(&f.reason, "reason", "", "")
	fs.StringVar(&f.dataDir, "data-dir", "", "")
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 {
		return f, errors.New(upgradeNoteUsage)
	}
	f.by, f.reason = strings.TrimSpace(f.by), strings.TrimSpace(f.reason)
	switch {
	case f.by == "":
		return f, fmt.Errorf("--by is required: %s", upgradeNoteUsage)
	case len([]rune(f.by)) > upgradeNoteMaxByLen || strings.IndexFunc(f.by, unicode.IsControl) >= 0:
		return f, fmt.Errorf("--by must be at most %d characters with no control characters", upgradeNoteMaxByLen)
	case len([]rune(f.reason)) > upgradeNoteMaxReason || strings.IndexFunc(f.reason, unicode.IsControl) >= 0:
		return f, fmt.Errorf("--reason must be at most %d characters with no control characters", upgradeNoteMaxReason)
	}
	switch f.method {
	case upgradehistory.MethodManual, upgradehistory.MethodPackage, upgradehistory.MethodCI:
	default:
		return f, fmt.Errorf("--method must be manual, package or ci, got %q", f.method)
	}
	if f.dataDir == "" {
		f.dataDir = dataDirFromEnv()
	}
	return f, nil
}

// runUpgradeNote implements "levelrail upgrade-note": it records who swapped
// the binary by hand, for the next boot to put in upgrade history. The marker
// names this binary's own version, so run it after replacing the file and
// before the restart.
func runUpgradeNote(args []string, stdout io.Writer, now time.Time) error {
	f, err := parseUpgradeNoteFlags(args)
	if err != nil {
		return err
	}
	if info, statErr := os.Stat(f.dataDir); statErr != nil || !info.IsDir() {
		return fmt.Errorf("data dir %s does not exist; pass --data-dir or set APP_DATA_DIR", f.dataDir)
	}
	m := upgradehistory.Marker{
		ToVersion: version.Version, Initiator: f.by, Method: f.method, Reason: f.reason, WrittenAt: now.UTC(),
	}
	if err := upgradehistory.WriteMarker(f.dataDir, m); err != nil {
		return fmt.Errorf("record upgrade note: %w", err)
	}
	_, err = fmt.Fprintf(stdout, "recorded: %s will show as %q (%s) on the next start\n", version.Version, f.by, f.method)
	return err
}
