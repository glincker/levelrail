package main

import "github.com/GLINCKER/levelrail/internal/apiclient"

// cliProg is cmd/levelrail-cli's own conventional binary name (the
// basename `go build`/`go run` give it absent an explicit -o): checked
// as a second credentials-file location, after this binary's own, so an
// operator who already ran "levelrail-cli auth login" doesn't have to
// configure this server separately. If the CLI binary was renamed at
// build time, this fallback simply finds nothing, and the flag/env-var
// path (identical names, apiclient.EnvAPIToken/EnvAPIURL) still works.
const cliProg = "levelrail-cli"

// resolveProfile picks the active profile name by precedence:
// --profile flag, then APP_PROFILE, then apiclient.DefaultProfile.
func resolveProfile(flagProfile string, lookupEnv func(string) (string, bool)) string {
	return apiclient.ResolveProfile(flagProfile, lookupEnv)
}

// resolveToken picks an API token by precedence: flagToken, then
// APP_API_TOKEN, then this server's own credentials file, then the
// CLI's, both read under the same resolved profile.
func resolveToken(flagToken string, lookupEnv func(string) (string, bool), prog, profile string) string {
	if flagToken != "" {
		return flagToken
	}
	if v, ok := lookupEnv(apiclient.EnvAPIToken); ok && v != "" {
		return v
	}
	for _, p := range []string{prog, cliProg} {
		if creds, err := apiclient.ReadCredentialsFile(p, profile); err == nil && creds.Token != "" {
			return creds.Token
		}
	}
	return ""
}

// resolveAPIURL picks the base API URL by precedence: flagURL, then
// APP_API_URL, then this server's own credentials file, then the CLI's,
// then apiclient.DefaultAPIURL.
func resolveAPIURL(flagURL string, lookupEnv func(string) (string, bool), prog, profile string) string {
	if flagURL != "" {
		return flagURL
	}
	if v, ok := lookupEnv(apiclient.EnvAPIURL); ok && v != "" {
		return v
	}
	for _, p := range []string{prog, cliProg} {
		if creds, err := apiclient.ReadCredentialsFile(p, profile); err == nil && creds.APIURL != "" {
			return creds.APIURL
		}
	}
	return apiclient.DefaultAPIURL
}
