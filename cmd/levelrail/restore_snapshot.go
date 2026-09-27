package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/store"
)

const restoreSnapshotUsage = `usage: levelrail restore-snapshot [--list] [--dry-run] [--yes] [name | latest]

Rolls the control plane database back to a local snapshot, such as the one
taken automatically before a migration. Stop the control plane first. The
current database is kept beside it as a .before-restore copy.

Flags:
  --list      list local snapshots (newest first) and exit
  --dry-run   verify the snapshot and print what would change; touch nothing
  --yes       skip the confirmation prompt
`

// runRestoreSnapshot implements "levelrail restore-snapshot": pick a local
// snapshot, verify it, confirm, then swap it in via runRestoreDB.
func runRestoreSnapshot(ctx context.Context, args []string, dataDir string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("restore-snapshot", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	list := fs.Bool("list", false, "")
	dryRun := fs.Bool("dry-run", false, "")
	yes := fs.Bool("yes", false, "")
	if err := fs.Parse(args); err != nil {
		_, _ = fmt.Fprint(stdout, restoreSnapshotUsage)
		return errors.New("invalid flags")
	}
	mgr := cpbackup.NewManager(nil, dataDir)
	snaps, err := mgr.List()
	if err != nil {
		return fmt.Errorf("list snapshots: %w", err)
	}
	if *list {
		printSnapshots(stdout, snaps)
		return nil
	}
	if fs.NArg() > 1 {
		_, _ = fmt.Fprint(stdout, restoreSnapshotUsage)
		return errors.New("at most one snapshot name")
	}
	if len(snaps) == 0 {
		return errors.New("no local snapshots found")
	}
	name := "latest"
	if fs.NArg() == 1 {
		name = fs.Arg(0)
	}
	chosen, err := pickSnapshot(snaps, name)
	if err != nil {
		return err
	}
	path := filepath.Join(dataDir, cpbackup.DirName, chosen.Name)
	version, err := store.InspectSnapshot(ctx, path)
	if err != nil {
		return fmt.Errorf("snapshot rejected: %w", err)
	}
	latest, err := store.MaxSchemaVersion()
	if err != nil {
		return err
	}
	if version > latest {
		return fmt.Errorf("snapshot is at schema version %d, newer than the %d this binary supports; use a newer release", version, latest)
	}
	_, _ = fmt.Fprintf(stdout, "snapshot %s (schema version %d, %d bytes, taken %s)\n", chosen.Name, version, chosen.SizeBytes, chosen.CreatedAt.UTC().Format("2006-01-02 15:04:05Z"))
	if *dryRun {
		_, _ = fmt.Fprintln(stdout, "dry run: snapshot verified, nothing changed. Re-run without --dry-run to restore it.")
		return nil
	}
	if !*yes {
		_, _ = fmt.Fprint(stdout, "Replace the live database with this snapshot? Data written after it is lost. Type yes to continue: ")
		answer, _ := bufio.NewReader(stdin).ReadString('\n')
		if strings.TrimSpace(strings.ToLower(answer)) != "yes" {
			return errors.New("aborted, nothing changed")
		}
	}
	return runRestoreDB(ctx, []string{path}, dataDir, stdout)
}

func pickSnapshot(snaps []cpbackup.Info, name string) (cpbackup.Info, error) {
	if name == "latest" {
		return snaps[0], nil
	}
	for _, s := range snaps {
		if s.Name == name {
			return s, nil
		}
	}
	return cpbackup.Info{}, fmt.Errorf("no local snapshot named %q (use --list)", name)
}

func printSnapshots(out io.Writer, snaps []cpbackup.Info) {
	if len(snaps) == 0 {
		_, _ = fmt.Fprintln(out, "no local snapshots")
		return
	}
	for _, s := range snaps {
		_, _ = fmt.Fprintf(out, "%s\t%d bytes\t%s\n", s.Name, s.SizeBytes, s.CreatedAt.UTC().Format("2006-01-02 15:04:05Z"))
	}
}
