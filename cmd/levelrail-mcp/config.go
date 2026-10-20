package main

import "github.com/GLINCKER/levelrail/internal/apiclient"

// Fallback to the CLI's default credentials file. A renamed CLI binary finds nothing here, but the flags and env vars still work.
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
