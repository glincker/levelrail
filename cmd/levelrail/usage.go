package main

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/GLINCKER/levelrail/internal/version"
)

// serverSubcommands is every first argument main dispatches before it would
// start the server; anything else must never fall through to run().
var serverSubcommands = map[string]bool{
	"version": true, "retain": true, "upgrade-note": true, "rollback": true,
	"self-upgrade": true, "restore-point": true, "migrate-check": true, "recover-admin": true,
	"emergency-token": true, "approve-signin": true, "restore-db": true, "restore-snapshot": true,
	"restore": true, "setup-token": true, "healthcheck": true, "demo": true, "access-policy-reset": true,
}

const serverUsageBody = `Usage:
  %[1]s                     start the control plane (configured by APP_* environment variables)
  %[1]s <command> [args]

Commands:
  version            print the version
  healthcheck        probe the running control plane (--ready for readiness)
  demo               seed, serve or check the read-only demo instance (seed, serve, check)
  setup-token        print the first-run setup token
  recover-admin      reset an administrator account
  emergency-token    mint a short-lived emergency access token
  approve-signin     approve a waiting new-browser sign-in from the host (<request id> --match <number>)
  access-policy-reset  remove every session policy (IP allowlist lockout recovery)
  restore-db         restore the database from a backup file
  restore-snapshot   restore a full snapshot
  restore            restore from a backup target
  rollback           roll the binary back to the previous release
  self-upgrade       upgrade this binary in place
  restore-point      restore a pre-upgrade restore point (binary, unit file, database if needed)
  upgrade-note       show release notes for the pending upgrade
  retain             manage retained releases
  migrate-check      check pending schema migrations

Flags:
  -h, --help         print this help and exit
  -v, --version      print the version and exit
`

// bootArgsExit decides what main does with args before starting a server.
// handled is true when the process must exit with code without side effects.
func bootArgsExit(args []string, argv0 string, stdout, stderr io.Writer) (code int, handled bool) {
	if len(args) == 0 {
		return 0, false
	}
	name := filepath.Base(argv0)
	switch first := args[0]; {
	case first == "help" || first == "-h" || first == "--help" || first == "-help":
		_, _ = fmt.Fprintf(stdout, serverUsageBody, name)
		return 0, true
	case first == "-v" || first == "--version" || first == "-version":
		_, _ = fmt.Fprintln(stdout, version.Version)
		return 0, true
	case serverSubcommands[first]:
		return 0, false
	default:
		_, _ = fmt.Fprintf(stderr, "unknown command or flag %q\n\n", first)
		_, _ = fmt.Fprintf(stderr, serverUsageBody, name)
		return 2, true
	}
}
