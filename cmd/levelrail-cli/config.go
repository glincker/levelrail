package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const (
	envAPIToken         = apiclient.EnvAPIToken
	envAPIURL           = apiclient.EnvAPIURL
	envProfile          = apiclient.EnvProfile
	defaultAPIURL       = apiclient.DefaultAPIURL
	defaultProfile      = apiclient.DefaultProfile
	credentialsFileName = apiclient.CredentialsFileName
)

type credentials = apiclient.Credentials
type profileSummary = apiclient.ProfileSummary

// resolveProfile picks the active profile name by precedence: --profile
// flag, then APP_PROFILE, then defaultProfile. See apiclient.ResolveProfile.
func resolveProfile(flagProfile string, lookupEnv func(string) (string, bool)) string {
	return apiclient.ResolveProfile(flagProfile, lookupEnv)
}

// resolveToken picks an API token by precedence: --token flag, then
// APP_API_TOKEN, then profile's section of the local credentials file.
// See apiclient.ResolveToken.
func resolveToken(flagToken string, lookupEnv func(string) (string, bool), prog, profile string) string {
	return apiclient.ResolveToken(flagToken, lookupEnv, prog, profile)
}

// resolveAPIURL picks the base API URL by precedence: --api-url flag,
// then APP_API_URL, then profile's section of the credentials file,
// then defaultAPIURL. See apiclient.ResolveAPIURL.
func resolveAPIURL(flagURL string, lookupEnv func(string) (string, bool), prog, profile string) string {
	return apiclient.ResolveAPIURL(flagURL, lookupEnv, prog, profile)
}

// configDir is "~/.config/<prog>". See apiclient.ConfigDir.
func configDir(prog string) (string, error) {
	return apiclient.ConfigDir(prog)
}

// readCredentialsFile reads profile's section of prog's credentials
// file. See apiclient.ReadCredentialsFile.
func readCredentialsFile(prog, profile string) (credentials, error) {
	return apiclient.ReadCredentialsFile(prog, profile)
}

// writeCredentialsFile writes creds under profile's section of prog's
// credentials file. See apiclient.WriteCredentialsFile.
func writeCredentialsFile(prog, profile string, creds credentials) error {
	return apiclient.WriteCredentialsFile(prog, profile, creds)
}

// listProfiles lists every profile configured in prog's credentials
// file. See apiclient.ListProfiles.
func listProfiles(prog string) ([]profileSummary, error) {
	return apiclient.ListProfiles(prog)
}

// apiURLEnvHelp names both env vars in help text; LEVELRAIL_API_URL wins when both are set.
const apiURLEnvHelp = envLevelrailAPIURL + " or " + envAPIURL

// apiTokenEnvHelp names both token env vars in help text; LEVELRAIL_TOKEN wins when both are set.
const apiTokenEnvHelp = envLevelrailToken + " or " + envAPIToken

var profileNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func validateProfileName(name string) error {
	if !profileNamePattern.MatchString(name) {
		return newValidationError("invalid profile name %q: use letters, digits, dot, dash or underscore", name)
	}
	return nil
}

type credBlock struct {
	name  string
	lines []string
}

func credHeaderName(line string) (string, bool) {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "[") || !strings.HasSuffix(t, "]") {
		return "", false
	}
	if name := strings.TrimSpace(t[1 : len(t)-1]); name != "" {
		return name, true
	}
	return defaultProfile, true
}

// splitCredBlocks keeps unknown keys and comments intact, and reads a flat
// pre-profile file as the default profile so edits never drop its keys.
func splitCredBlocks(text string) (pre []string, blocks []credBlock) {
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if name, ok := credHeaderName(line); ok {
			blocks = append(blocks, credBlock{name: name, lines: []string{"[" + name + "]"}})
			continue
		}
		t := strings.TrimSpace(line)
		if len(blocks) == 0 {
			if strings.Contains(t, "=") && !strings.HasPrefix(t, "#") {
				blocks = append(blocks, credBlock{name: defaultProfile, lines: []string{"[" + defaultProfile + "]", line}})
			} else {
				pre = append(pre, line)
			}
			continue
		}
		last := &blocks[len(blocks)-1]
		last.lines = append(last.lines, line)
	}
	return pre, blocks
}

func editCredentialsBlocks(prog string, edit func(blocks []credBlock) ([]credBlock, error)) error {
	dir, err := configDir(prog)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, credentialsFileName)
	data, err := os.ReadFile(path) //nolint:gosec // fixed file name under the config dir
	if err != nil {
		if os.IsNotExist(err) {
			return newValidationError("no credentials file yet, run \"%s auth login\" first", prog)
		}
		return fmt.Errorf("read credentials file: %w", err)
	}
	pre, blocks := splitCredBlocks(string(data))
	blocks, err = edit(blocks)
	if err != nil {
		return err
	}
	out := append([]string{}, pre...)
	for _, b := range blocks {
		out = append(out, b.lines...)
	}
	tmp, err := os.CreateTemp(dir, ".credentials-*")
	if err != nil {
		return fmt.Errorf("write credentials file: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write credentials file: %w", err)
	}
	if _, err := tmp.WriteString(strings.Join(out, "\n") + "\n"); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write credentials file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write credentials file: %w", err)
	}
	return os.Rename(tmp.Name(), path)
}

func profileExists(prog, name string) (bool, error) {
	profiles, err := listProfiles(prog)
	if err != nil {
		return false, err
	}
	for _, p := range profiles {
		if p.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// removeProfile drops the credentials section, its saved team/project and a current-context pointing at it.
func removeProfile(prog, name string) error {
	ok, err := profileExists(prog, name)
	if err != nil {
		return err
	}
	if !ok {
		return newValidationError("no profile %q, run \"%s profile list\" to see them", name, prog)
	}
	if err := editCredentialsBlocks(prog, func(blocks []credBlock) ([]credBlock, error) {
		kept := blocks[:0:0]
		for _, b := range blocks {
			if b.name != name {
				kept = append(kept, b)
			}
		}
		return kept, nil
	}); err != nil {
		return err
	}
	_ = apiclient.WriteContext(prog, name, apiclient.Scope{})
	if readCurrentContext(prog) == name {
		return writeCurrentContext(prog, "")
	}
	return nil
}

// renameProfile moves the credentials section, the saved scope and the current-context pointer together.
func renameProfile(prog, from, to string) error {
	if err := validateProfileName(to); err != nil {
		return err
	}
	ok, err := profileExists(prog, from)
	if err != nil {
		return err
	}
	if !ok {
		return newValidationError("no profile %q, run \"%s profile list\" to see them", from, prog)
	}
	if exists, _ := profileExists(prog, to); exists {
		return newValidationError("profile %q already exists, remove it first", to)
	}
	if err := editCredentialsBlocks(prog, func(blocks []credBlock) ([]credBlock, error) {
		for i := range blocks {
			if blocks[i].name == from {
				blocks[i].name = to
				blocks[i].lines[0] = "[" + to + "]"
			}
		}
		return blocks, nil
	}); err != nil {
		return err
	}
	if sc, _ := apiclient.ReadContext(prog, from); sc.Team != "" || sc.Project != "" {
		_ = apiclient.WriteContext(prog, to, sc)
		_ = apiclient.WriteContext(prog, from, apiclient.Scope{})
	}
	if readCurrentContext(prog) == from {
		return writeCurrentContext(prog, to)
	}
	return nil
}

const (
	settingsFileName = "settings"
	settingOutput    = "output"
)

var validDefaultOutputs = map[string]bool{"json": true, "ndjson": true, "yaml": true, "table": true, "wide": true, "text": true, "name": true}

// readSetting reads one key from the plain key=value settings file; a missing file or key is empty.
func readSetting(prog, key string) string {
	dir, err := configDir(prog)
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(dir, settingsFileName)) //nolint:gosec // fixed file name under the config dir
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok && strings.TrimSpace(k) == key {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// writeSetting rewrites the settings file with key set, or removed when value is empty, keeping other keys.
func writeSetting(prog, key, value string) error {
	dir, err := configDir(prog)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	path := filepath.Join(dir, settingsFileName)
	var kept []string
	if data, rerr := os.ReadFile(path); rerr == nil { //nolint:gosec // fixed file name under the config dir
		for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
			if k, _, ok := strings.Cut(strings.TrimSpace(line), "="); ok && strings.TrimSpace(k) == key {
				continue
			}
			if strings.TrimSpace(line) != "" {
				kept = append(kept, line)
			}
		}
	}
	if value != "" {
		kept = append(kept, key+"="+value)
	}
	return os.WriteFile(path, []byte(strings.Join(kept, "\n")+"\n"), 0o600) //nolint:gosec // fixed file name under the config dir
}

func runConfigSet(prog string, args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 || args[0] != settingOutput {
		_, _ = fmt.Fprint(stderr, configUsage(prog))
		return exitUsage
	}
	value := args[1]
	if value == "default" || value == "unset" {
		value = ""
	} else if !validDefaultOutputs[value] {
		_, _ = fmt.Fprintf(stderr, "%s: invalid output %q, want json, ndjson, yaml, table, wide, text, name or default\n", prog, value)
		return exitValidation
	}
	if err := writeSetting(prog, settingOutput, value); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if value == "" {
		_, _ = fmt.Fprintln(stdout, "Default output cleared.")
	} else {
		_, _ = fmt.Fprintf(stdout, "Default output set to %s.\n", value)
	}
	return exitOK
}

func runConfigGet(prog string, args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 || args[0] != settingOutput {
		_, _ = fmt.Fprint(stderr, configUsage(prog))
		return exitUsage
	}
	v := readSetting(prog, settingOutput)
	if v == "" {
		v = "table"
	}
	_, _ = fmt.Fprintln(stdout, v)
	return exitOK
}
