package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const keyLossNote = "Losing the backup encryption key means encrypted backups cannot be restored. Keep a copy of backup-encryption.key off this machine."

// runBackupsEncryption implements "backups encryption get|set <destination> [on|off]".
func runBackupsEncryption(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	usage := fmt.Sprintf(`Usage:
  %[1]s backups encryption get <destination> [flags]
  %[1]s backups encryption set <destination> on|off [flags]

Shows or changes whether new database dumps sent to a backup destination are
encrypted with age. Existing dumps are never rewritten: old unencrypted dumps
stay as they are and still restore. Changing the setting needs a root token.
%[2]s
`, prog, keyLossNote)
	if len(args) == 0 || (args[0] != "get" && args[0] != "set") {
		_, _ = fmt.Fprint(stderr, usage)
		return exitUsage
	}
	verb := args[0]
	want := 1
	if verb == "set" {
		want = 2
	}
	c := newProtectionCmd(prog, "backups encryption "+verb, usage, stdout, stderr, lookupEnv)
	rest, exit, ok := c.parse(args[1:], want)
	if !ok {
		return exit
	}
	var res apiclient.BackupTargetEncryptionResource
	var err error
	if verb == "get" {
		res, err = c.client.GetBackupTargetEncryption(context.Background(), rest[0])
	} else {
		var enabled bool
		switch rest[1] {
		case "on":
			enabled = true
		case "off":
		default:
			_, _ = fmt.Fprintf(stderr, "%s: expected on or off, got %q\n", prog, rest[1])
			return exitUsage
		}
		res, err = c.client.SetBackupTargetEncryption(context.Background(), rest[0], enabled)
	}
	if err != nil {
		return c.fail(fmt.Errorf("backup encryption for %q: %w", rest[0], err))
	}
	return c.render(res, func() {
		_, _ = fmt.Fprintf(stdout, "dump encryption: %t\n", res.Enabled)
		_, _ = fmt.Fprintf(stdout, "key available:   %t\n", res.KeyAvailable)
		if res.Recipient != "" {
			_, _ = fmt.Fprintf(stdout, "public key:      %s\n", res.Recipient)
			_, _ = fmt.Fprintf(stdout, "key copy kept off this server: %t\n", res.KeyStoredOffBox)
		}
		_, _ = fmt.Fprintln(stdout, keyLossNote)
	})
}
