package main

import (
	"os"
	"strings"
)

const (
	envLevelrailToken   = "LEVELRAIL_TOKEN" //nolint:gosec // env var name, not a credential
	envLevelrailAPIURL  = "LEVELRAIL_API_URL"
	envLevelrailProfile = "LEVELRAIL_PROFILE"
	envLevelrailCI      = "LEVELRAIL_CI"
	envNoColor          = "NO_COLOR"
)

// ciMode is set once per run(): never prompt, no color, no animation.
var ciMode bool

// forceCI is set by main() when stdin or stdout is not a terminal.
var forceCI bool

// rawLookupEnv is the unaliased lookup, kept so credentialSource can tell which name supplied a value.
var rawLookupEnv = os.LookupEnv

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes":
		return true
	}
	return false
}

func ciEnv(lookup func(string) (string, bool)) bool {
	if v, ok := lookup("CI"); ok && truthy(v) {
		return true
	}
	v, ok := lookup(envLevelrailCI)
	return ok && truthy(v)
}

// colorDisabled honors NO_COLOR (any non-empty value, per no-color.org) and CI mode.
func colorDisabled(lookup func(string) (string, bool), ci bool) bool {
	if v, ok := lookup(envNoColor); ok && v != "" {
		return true
	}
	return ci
}

// envAliasLookup lets LEVELRAIL_* win over the legacy APP_* names without touching their readers.
func envAliasLookup(lookup func(string) (string, bool)) func(string) (string, bool) {
	alias := map[string]string{
		envAPIToken: envLevelrailToken,
		envAPIURL:   envLevelrailAPIURL,
		envProfile:  envLevelrailProfile,
	}
	return func(key string) (string, bool) {
		if newName, ok := alias[key]; ok {
			if v, set := lookup(newName); set && v != "" {
				return v, true
			}
		}
		return lookup(key)
	}
}

// requireInput fails fast in CI mode, naming what to pass instead of prompting.
func requireInput(what, fix string) error {
	if !ciMode {
		return nil
	}
	return newValidationError("%s needs input but CI mode never prompts: %s", what, fix)
}

// credentialSource names where the token came from, never the token itself.
func credentialSource(flagToken string, prog, profile string) string {
	if flagToken != "" {
		return "flag --token"
	}
	for _, name := range []string{envLevelrailToken, envAPIToken} {
		if v, ok := rawLookupEnv(name); ok && v != "" {
			return "env " + name
		}
	}
	if c, err := readCredentialsFile(prog, profile); err == nil && c.Token != "" {
		return "profile " + profile
	}
	return "none"
}
