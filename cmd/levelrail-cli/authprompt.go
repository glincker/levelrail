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
		// Refuse at EOF instead of hanging; this is a usage error, not a network failure.
		return "", newValidationError("read username: %v", err)
	}
	return strings.TrimSpace(line), nil
}

// readPassword is injected so tests can stub the terminal read.
func resolveLoginCredentials(usernameFlag, passwordFlag string, stdin io.Reader, stderr io.Writer, readPassword func() (string, error)) (username, password string, err error) {
	username = usernameFlag
	if username == "" {
		if err := requireInput("login", "pass --username and --password, or set "+envLevelrailToken+" and skip login"); err != nil {
			return "", "", err
		}
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
		if err := requireInput("login", "pass --password"); err != nil {
			return "", "", err
		}
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
