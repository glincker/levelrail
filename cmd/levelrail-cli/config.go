package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// defaultAPIURL is the sensible localhost default the competitive
// research direction calls for (flyctl/railway/vercel all ship a
// zero-config local default): the control plane's own defaultHTTPAddr
// (cmd/levelrail/main.go) is ":8080", so this is that same port on
// loopback.
const defaultAPIURL = "http://localhost:8080"

// envAPIToken and envAPIURL are this project's established APP_*
// env-var-prefix convention (see internal/brand's own envPrefix, and
// cmd/levelrail/main.go's APP_DATA_DIR/APP_HTTP_ADDR/etc.), not a
// product-name-specific prefix: renaming the product later must not
// require renaming these.
const (
	envAPIToken = "APP_API_TOKEN" //nolint:gosec // this is the name of an env var, not a credential value
	envAPIURL   = "APP_API_URL"
)

// credentialsFileName is the file resolveCredentialsFile reads, inside
// a directory named after the running binary (configDir below), not a
// hardcoded product name: the brand-indirection rule applies here
// exactly as it does to the control plane itself, and "CLI command name
// comes from os.Args[0]" is the same mechanism this reuses for its own
// config directory.
const credentialsFileName = "credentials"

// resolveToken picks an API token by precedence: --token flag, then
// APP_API_TOKEN, then the local credentials file. Returns "" (not an
// error) if none is set: an empty token is a valid, if unlikely to
// succeed, thing to send, and letting the server's own 401 be what
// reports "not authenticated" keeps this function from duplicating that
// judgment.
func resolveToken(flagToken string, lookupEnv func(string) (string, bool), prog string) string {
	if flagToken != "" {
		return flagToken
	}
	if v, ok := lookupEnv(envAPIToken); ok && v != "" {
		return v
	}
	if creds, err := readCredentialsFile(prog); err == nil && creds.token != "" {
		return creds.token
	}
	return ""
}

// resolveAPIURL picks the base API URL by precedence: --api-url flag,
// then APP_API_URL, then the credentials file, then defaultAPIURL.
func resolveAPIURL(flagURL string, lookupEnv func(string) (string, bool), prog string) string {
	if flagURL != "" {
		return flagURL
	}
	if v, ok := lookupEnv(envAPIURL); ok && v != "" {
		return v
	}
	if creds, err := readCredentialsFile(prog); err == nil && creds.apiURL != "" {
		return creds.apiURL
	}
	return defaultAPIURL
}

type credentials struct {
	apiURL string
	token  string
}

// configDir is "~/.config/<prog>", prog being the running binary's own
// basename (filepath.Base(os.Args[0])), never a hardcoded product name:
// this makes the credentials file location follow the binary
// automatically if it's ever renamed, the same rebrandability property
// required everywhere else in this codebase.
func configDir(prog string) (string, error) {
	if prog == "" {
		prog = "cli"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config", prog), nil
}

// readCredentialsFile reads a minimal "key=value" per line file (the
// same two keys as the env vars: APP_API_URL, APP_API_TOKEN), so a
// human can `cat` or hand-edit it without needing to know a
// CLI-specific format. A missing file is a plain error the caller
// treats as "no credentials file," not logged or reported: this is the
// lowest-priority source in the precedence chain, its absence is the
// common case, not a problem.
func readCredentialsFile(prog string) (credentials, error) {
	dir, err := configDir(prog)
	if err != nil {
		return credentials{}, err
	}
	path := filepath.Join(dir, credentialsFileName)

	f, err := os.Open(path) //nolint:gosec // fixed, user-controlled config path derived from the binary's own name, not request input
	if err != nil {
		return credentials{}, err
	}
	defer func() { _ = f.Close() }()

	var creds credentials
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case envAPIURL:
			creds.apiURL = value
		case envAPIToken:
			creds.token = value
		}
	}
	if err := scanner.Err(); err != nil {
		return credentials{}, fmt.Errorf("read credentials file %s: %w", path, err)
	}
	return creds, nil
}

// writeCredentialsFile writes creds to the same "key=value" file
// readCredentialsFile reads, creating configDir(prog) if it doesn't
// exist yet. "auth login" is this function's only caller: it is the
// one command in this CLI that produces a credential rather than just
// consuming one. 0o600 on both the directory and the file, tighter than
// readCredentialsFile's own comment on the file needing to stay
// hand-editable: a freshly minted API token is a live credential, not a
// value that should ever be group- or world-readable by default.
func writeCredentialsFile(prog string, creds credentials) error {
	dir, err := configDir(prog)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config directory %s: %w", dir, err)
	}
	path := filepath.Join(dir, credentialsFileName)

	var sb strings.Builder
	sb.WriteString("# written by \"" + prog + " auth login\"\n")
	if creds.apiURL != "" {
		sb.WriteString(envAPIURL + "=" + creds.apiURL + "\n")
	}
	if creds.token != "" {
		sb.WriteString(envAPIToken + "=" + creds.token + "\n")
	}

	if err := os.WriteFile(path, []byte(sb.String()), 0o600); err != nil {
		return fmt.Errorf("write credentials file %s: %w", path, err)
	}
	return nil
}
