package apiclient

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultAPIURL is a zero-config local default, so a caller with no
// flag, env var, or credentials file still reaches a locally running
// control plane out of the box: its own defaultHTTPAddr
// (cmd/levelrail/main.go) is ":8080", so this is that same port on
// loopback.
const DefaultAPIURL = "http://localhost:8080"

// EnvAPIToken and EnvAPIURL are this project's established APP_*
// env-var-prefix convention (see internal/brand's own envPrefix, and
// cmd/levelrail/main.go's APP_DATA_DIR/APP_HTTP_ADDR/etc.), not a
// product-name-specific prefix: renaming the product later must not
// require renaming these. Every caller of this package (the CLI, the
// MCP server) uses these same two names, so a token or URL set for one
// is already set for the other.
const (
	EnvAPIToken = "APP_API_TOKEN" //nolint:gosec // this is the name of an env var, not a credential value
	EnvAPIURL   = "APP_API_URL"
)

// CredentialsFileName is the file ReadCredentialsFile reads, inside a
// directory named after the caller's own binary (ConfigDir below), not
// a hardcoded product name.
const CredentialsFileName = "credentials"

// ResolveToken picks an API token by precedence: flagToken, then
// EnvAPIToken, then the local credentials file. Returns "" (not an
// error) if none is set: an empty token is a valid, if unlikely to
// succeed, thing to send, and letting the server's own 401 be what
// reports "not authenticated" keeps this function from duplicating that
// judgment.
func ResolveToken(flagToken string, lookupEnv func(string) (string, bool), prog string) string {
	if flagToken != "" {
		return flagToken
	}
	if v, ok := lookupEnv(EnvAPIToken); ok && v != "" {
		return v
	}
	if creds, err := ReadCredentialsFile(prog); err == nil && creds.Token != "" {
		return creds.Token
	}
	return ""
}

// ResolveAPIURL picks the base API URL by precedence: flagURL, then
// EnvAPIURL, then the credentials file, then DefaultAPIURL.
func ResolveAPIURL(flagURL string, lookupEnv func(string) (string, bool), prog string) string {
	if flagURL != "" {
		return flagURL
	}
	if v, ok := lookupEnv(EnvAPIURL); ok && v != "" {
		return v
	}
	if creds, err := ReadCredentialsFile(prog); err == nil && creds.APIURL != "" {
		return creds.APIURL
	}
	return DefaultAPIURL
}

// Credentials is ReadCredentialsFile/WriteCredentialsFile's in-memory
// shape: the same two values as EnvAPIURL/EnvAPIToken.
type Credentials struct {
	APIURL string
	Token  string
}

// ConfigDir is "~/.config/<prog>", prog being the caller's own binary
// basename (filepath.Base(os.Args[0])), never a hardcoded product name:
// this makes the credentials file location follow the binary
// automatically if it's ever renamed, the same rebrandability property
// required everywhere else in this codebase.
func ConfigDir(prog string) (string, error) {
	if prog == "" {
		prog = "cli"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config", prog), nil
}

// ReadCredentialsFile reads a minimal "key=value" per line file (the
// same two keys as the env vars: APP_API_URL, APP_API_TOKEN), so a
// human can `cat` or hand-edit it without needing to know a
// caller-specific format. A missing file is a plain error the caller
// treats as "no credentials file," not logged or reported: this is the
// lowest-priority source in the precedence chain, its absence is the
// common case, not a problem.
func ReadCredentialsFile(prog string) (Credentials, error) {
	dir, err := ConfigDir(prog)
	if err != nil {
		return Credentials{}, err
	}
	path := filepath.Join(dir, CredentialsFileName)

	f, err := os.Open(path) //nolint:gosec // fixed, caller-controlled config path derived from the binary's own name, not request input
	if err != nil {
		return Credentials{}, err
	}
	defer func() { _ = f.Close() }()

	var creds Credentials
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
		case EnvAPIURL:
			creds.APIURL = value
		case EnvAPIToken:
			creds.Token = value
		}
	}
	if err := scanner.Err(); err != nil {
		return Credentials{}, fmt.Errorf("read credentials file %s: %w", path, err)
	}
	return creds, nil
}

// WriteCredentialsFile writes creds to the same "key=value" file
// ReadCredentialsFile reads, creating ConfigDir(prog) if it doesn't
// exist yet. "auth login" is this function's only caller today: it is
// the one command that produces a credential rather than just consuming
// one. 0o600 on both the directory and the file, tighter than
// ReadCredentialsFile's own comment on the file needing to stay
// hand-editable: a freshly minted API token is a live credential, not a
// value that should ever be group- or world-readable by default.
func WriteCredentialsFile(prog string, creds Credentials) error {
	dir, err := ConfigDir(prog)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config directory %s: %w", dir, err)
	}
	path := filepath.Join(dir, CredentialsFileName)

	var sb strings.Builder
	sb.WriteString("# written by \"" + prog + " auth login\"\n")
	if creds.APIURL != "" {
		sb.WriteString(EnvAPIURL + "=" + creds.APIURL + "\n")
	}
	if creds.Token != "" {
		sb.WriteString(EnvAPIToken + "=" + creds.Token + "\n")
	}

	if err := os.WriteFile(path, []byte(sb.String()), 0o600); err != nil {
		return fmt.Errorf("write credentials file %s: %w", path, err)
	}
	return nil
}
