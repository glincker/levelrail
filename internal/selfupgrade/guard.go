package selfupgrade

import (
	"fmt"
	"strings"
)

// ExitDowngradeRefused is the process exit status when a binary refuses to
// start on a database written by a newer release. systemd can be told not to
// restart on it (RestartPreventExitStatus).
const ExitDowngradeRefused = 78

// GuardInput describes a binary that found a database newer than itself.
type GuardInput struct {
	BinaryVersion string
	BinarySchema  int
	DBSchema      int
	// DBVersion is the release that last ran on the database, "" if unknown.
	DBVersion string
	// Program is the executable's name, used in the recovery commands.
	Program string
	// InstallerCommand is the verified installer invocation prefix, for
	// example "curl -fsSL <url> | sudo".
	InstallerCommand string
}

// DowngradeMessage explains the refusal and gives the exact recovery
// commands. Migrations only go forward, so the database cannot be opened by
// an older binary, and the message says which release to run instead.
func DowngradeMessage(in GuardInput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "refusing to start: this binary (%s, schema %d) is older than the database (schema %d", in.BinaryVersion, in.BinarySchema, in.DBSchema)
	if in.DBVersion != "" {
		fmt.Fprintf(&b, ", last run by %s", in.DBVersion)
	}
	b.WriteString("). Migrations are forward-only, so nothing was changed and the data is intact.\n")
	target := in.DBVersion
	if target == "" {
		target = "the newest release"
	}
	fmt.Fprintf(&b, "Fix: run %s or newer.\n", target)
	if in.DBVersion != "" {
		fmt.Fprintf(&b, "  1. Install it: %s LEVELRAIL_VERSION=%s sh -s upgrade\n", in.InstallerCommand, in.DBVersion)
	} else {
		fmt.Fprintf(&b, "  1. Install it: %s sh -s upgrade\n", in.InstallerCommand)
	}
	fmt.Fprintf(&b, "  2. Or, if you must run %s, restore a backup taken by it: sudo %s restore-snapshot --list\n",
		in.BinaryVersion, in.Program)
	return b.String()
}
