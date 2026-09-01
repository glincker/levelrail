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

// EnvAPIToken, EnvAPIURL, and EnvProfile are this project's established
// APP_*-env-var-prefix convention (see internal/brand's own envPrefix,
// and cmd/levelrail/main.go's APP_DATA_DIR/APP_HTTP_ADDR/etc.), not a
// product-name-specific prefix: renaming the product later must not
// require renaming these. Every caller of this package (the CLI, the
// MCP server) uses these same names, so a token or URL set for one is
// already set for the other.
const (
	EnvAPIToken = "APP_API_TOKEN" //nolint:gosec // this is the name of an env var, not a credential value
	EnvAPIURL   = "APP_API_URL"
	EnvProfile  = "APP_PROFILE"
)

// CredentialsFileName is the file ReadCredentialsFile reads, inside a
// directory named after the caller's own binary (ConfigDir below), not
// a hardcoded product name.
const CredentialsFileName = "credentials"

// DefaultProfile is the section ResolveProfile falls back to when
// neither a --profile flag nor APP_PROFILE is set, and the section an
// old, pre-profile flat credentials file (no "[section]" headers at
// all) is read as, for backward compatibility with every credentials
// file written before profile support existed.
const DefaultProfile = "default"

// ResolveProfile picks the active profile name by precedence:
// flagProfile, then EnvProfile, then DefaultProfile. It never returns
// an empty string.
func ResolveProfile(flagProfile string, lookupEnv func(string) (string, bool)) string {
	if flagProfile != "" {
		return flagProfile
	}
	if v, ok := lookupEnv(EnvProfile); ok && v != "" {
		return v
	}
	return DefaultProfile
}

// ResolveToken picks an API token by precedence: flagToken, then
// EnvAPIToken, then profile's section of the local credentials file.
// Returns "" (not an error) if none is set: an empty token is a valid,
// if unlikely to succeed, thing to send, and letting the server's own
// 401 be what reports "not authenticated" keeps this function from
// duplicating that judgment.
func ResolveToken(flagToken string, lookupEnv func(string) (string, bool), prog, profile string) string {
	if flagToken != "" {
		return flagToken
	}
	if v, ok := lookupEnv(EnvAPIToken); ok && v != "" {
		return v
	}
	if creds, err := ReadCredentialsFile(prog, profile); err == nil && creds.Token != "" {
		return creds.Token
	}
	return ""
}

// ResolveAPIURL picks the base API URL by precedence: flagURL, then
// EnvAPIURL, then profile's section of the credentials file, then
// DefaultAPIURL.
func ResolveAPIURL(flagURL string, lookupEnv func(string) (string, bool), prog, profile string) string {
	if flagURL != "" {
		return flagURL
	}
	if v, ok := lookupEnv(EnvAPIURL); ok && v != "" {
		return v
	}
	if creds, err := ReadCredentialsFile(prog, profile); err == nil && creds.APIURL != "" {
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

// ProfileSummary is ListProfiles' element type: a profile's name and API
// URL, deliberately never its token. See ListProfiles.
type ProfileSummary struct {
	Name   string
	APIURL string
}

// parsedCredentialsFile is ReadCredentialsFile/WriteCredentialsFile's
// shared parse result: every section keyed by name, plus the order
// sections first appeared in the file, so WriteCredentialsFile can
// round-trip an existing file's other profiles untouched and
// ListProfiles can report profiles in a stable, file order.
type parsedCredentialsFile struct {
	sections map[string]Credentials
	order    []string
}

// parseCredentialsFile reads path as INI-style sections ("[name]"
// headers, "KEY=VALUE" lines under each), matching ~/.aws/credentials'
// own format. A file with no "[section]" header at all (every
// credentials file written before profile support existed) is read
// entirely as DefaultProfile, so an old flat file still loads correctly.
func parseCredentialsFile(path string) (parsedCredentialsFile, error) {
	f, err := os.Open(path) //nolint:gosec // fixed, caller-controlled config path derived from the binary's own name, not request input
	if err != nil {
		return parsedCredentialsFile{}, err
	}
	defer func() { _ = f.Close() }()

	parsed := parsedCredentialsFile{sections: map[string]Credentials{}}
	section := DefaultProfile
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if name, ok := sectionHeaderName(line); ok {
			section = name
			parsed.ensureSection(section)
			continue
		}
		parsed.setKeyValueLine(section, line)
	}
	if err := scanner.Err(); err != nil {
		return parsedCredentialsFile{}, fmt.Errorf("read credentials file %s: %w", path, err)
	}
	return parsed, nil
}

// sectionHeaderName reports whether line is an INI "[name]" section
// header, and if so, its name (DefaultProfile for an empty "[]").
func sectionHeaderName(line string) (string, bool) {
	if !strings.HasPrefix(line, "[") || !strings.HasSuffix(line, "]") {
		return "", false
	}
	name := strings.TrimSpace(line[1 : len(line)-1])
	if name == "" {
		name = DefaultProfile
	}
	return name, true
}

// ensureSection registers section in p's sections/order the first time
// it's seen, so an empty section (or one with no recognized key) still
// shows up in ListProfiles.
func (p *parsedCredentialsFile) ensureSection(section string) {
	if _, ok := p.sections[section]; !ok {
		p.sections[section] = Credentials{}
		p.order = append(p.order, section)
	}
}

// setKeyValueLine parses a "KEY=VALUE" line into section's Credentials,
// ignoring any key it doesn't recognize (forward-compatible with a
// future key added to the file) and any line with no "=" at all.
func (p *parsedCredentialsFile) setKeyValueLine(section, line string) {
	key, value, ok := strings.Cut(line, "=")
	if !ok {
		return
	}
	key, value = strings.TrimSpace(key), strings.TrimSpace(value)

	p.ensureSection(section)
	creds := p.sections[section]
	switch key {
	case EnvAPIURL:
		creds.APIURL = value
	case EnvAPIToken:
		creds.Token = value
	}
	p.sections[section] = creds
}

// ReadCredentialsFile reads profile's section from prog's credentials
// file. A missing file is a plain error the caller treats as "no
// credentials file," not logged or reported: this is the
// lowest-priority source in ResolveToken/ResolveAPIURL's precedence
// chain, its absence is the common case, not a problem. A file that
// exists but has no section named profile returns a zero Credentials
// and no error, the same "nothing configured here" shape, since asking
// for an unconfigured profile is not itself a failure to read the file.
func ReadCredentialsFile(prog, profile string) (Credentials, error) {
	dir, err := ConfigDir(prog)
	if err != nil {
		return Credentials{}, err
	}
	path := filepath.Join(dir, CredentialsFileName)

	parsed, err := parseCredentialsFile(path)
	if err != nil {
		return Credentials{}, err
	}
	return parsed.sections[profile], nil
}

// ListProfiles returns every profile section configured in prog's
// credentials file, in the order they first appear, each with its API
// URL but never its token: see ProfileSummary. Returns a nil slice, no
// error, for a missing file, the same "nothing configured" shape
// ReadCredentialsFile's own doc comment describes.
func ListProfiles(prog string) ([]ProfileSummary, error) {
	dir, err := ConfigDir(prog)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, CredentialsFileName)

	parsed, err := parseCredentialsFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	summaries := make([]ProfileSummary, 0, len(parsed.order))
	for _, name := range parsed.order {
		summaries = append(summaries, ProfileSummary{Name: name, APIURL: parsed.sections[name].APIURL})
	}
	return summaries, nil
}

// WriteCredentialsFile writes creds under profile's section in prog's
// credentials file, creating ConfigDir(prog) if it doesn't exist yet,
// and leaving every other profile section already in the file
// untouched. "auth login" is this function's only caller today: it is
// the one command that produces a credential rather than just consuming
// one. 0o600 on both the directory and the file, tighter than
// ReadCredentialsFile's own comment on the file needing to stay
// hand-editable: a freshly minted API token is a live credential, not a
// value that should ever be group- or world-readable by default.
func WriteCredentialsFile(prog, profile string, creds Credentials) error {
	dir, err := ConfigDir(prog)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config directory %s: %w", dir, err)
	}
	path := filepath.Join(dir, CredentialsFileName)

	parsed, err := parseCredentialsFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read existing credentials file %s: %w", path, err)
	}
	if parsed.sections == nil {
		parsed.sections = map[string]Credentials{}
	}
	if _, seen := parsed.sections[profile]; !seen {
		parsed.order = append(parsed.order, profile)
	}
	parsed.sections[profile] = creds

	var sb strings.Builder
	sb.WriteString("# written by \"" + prog + " auth login\"\n")
	for _, name := range parsed.order {
		c := parsed.sections[name]
		sb.WriteString("[" + name + "]\n")
		if c.APIURL != "" {
			sb.WriteString(EnvAPIURL + "=" + c.APIURL + "\n")
		}
		if c.Token != "" {
			sb.WriteString(EnvAPIToken + "=" + c.Token + "\n")
		}
		sb.WriteString("\n")
	}

	if err := os.WriteFile(path, []byte(sb.String()), 0o600); err != nil {
		return fmt.Errorf("write credentials file %s: %w", path, err)
	}
	return nil
}
