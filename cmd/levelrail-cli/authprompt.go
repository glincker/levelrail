package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// promptUsername writes prompt to stderr (this CLI's diagnostics stream,
// never stdout, so --json output is never polluted by it) and reads one
// line from stdin. Used only when --username is omitted.
func promptUsername(stdin io.Reader, stderr io.Writer) (string, error) {
	_, _ = fmt.Fprint(stderr, "Username: ")
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && line == "" {
		// No terminal attached / stdin at EOF: refuse cleanly rather
		// than hang, classified as a validationError (a usage
		// problem, "no credentials available"), not a network
		// failure, matching backups_restore.go's own
		// resolveRestoreConfirmation reasoning for the identical EOF
		// case.
		return "", newValidationError("read username: %v", err)
	}
	return strings.TrimSpace(line), nil
}

// resolveLoginCredentials fills in username/password from flags first,
// prompting for whatever is missing: a plain line read from stdin for
// username, readPassword (production: golang.org/x/term.ReadPassword
// over the real terminal fd, so a password is never echoed) for
// password. readPassword is a function value, not a direct call to
// term.ReadPassword, so this whole resolution is testable with a stub
// that returns a fixed password without needing a real terminal.
func resolveLoginCredentials(usernameFlag, passwordFlag string, stdin io.Reader, stderr io.Writer, readPassword func() (string, error)) (username, password string, err error) {
	username = usernameFlag
	if username == "" {
		username, err = promptUsername(stdin, stderr)
		if err != nil {
			return "", "", err
		}
	}
	if username == "" {
		return "", "", newValidationError("username is required")
	}

	password = passwordFlag
	if password == "" {
		_, _ = fmt.Fprint(stderr, "Password: ")
		password, err = readPassword()
		_, _ = fmt.Fprintln(stderr)
		if err != nil {
			return "", "", newValidationError("read password: %v", err)
		}
	}
	if password == "" {
		return "", "", newValidationError("password is required")
	}

	return username, password, nil
}
