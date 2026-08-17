package main

import "github.com/GLINCKER/levelrail/internal/apiclient"

// envAPIToken, envAPIURL, defaultAPIURL, and credentialsFileName alias
// internal/apiclient's exported constants of the same values: see
// client.go's own doc comment on why this file no longer defines its
// own credentials logic.
const (
	envAPIToken         = apiclient.EnvAPIToken
	envAPIURL           = apiclient.EnvAPIURL
	defaultAPIURL       = apiclient.DefaultAPIURL
	credentialsFileName = apiclient.CredentialsFileName
)

type credentials = apiclient.Credentials

// resolveToken picks an API token by precedence: --token flag, then
// APP_API_TOKEN, then the local credentials file. See apiclient.ResolveToken.
func resolveToken(flagToken string, lookupEnv func(string) (string, bool), prog string) string {
	return apiclient.ResolveToken(flagToken, lookupEnv, prog)
}

// resolveAPIURL picks the base API URL by precedence: --api-url flag,
// then APP_API_URL, then the credentials file, then defaultAPIURL. See
// apiclient.ResolveAPIURL.
func resolveAPIURL(flagURL string, lookupEnv func(string) (string, bool), prog string) string {
	return apiclient.ResolveAPIURL(flagURL, lookupEnv, prog)
}

// configDir is "~/.config/<prog>". See apiclient.ConfigDir.
func configDir(prog string) (string, error) {
	return apiclient.ConfigDir(prog)
}

// readCredentialsFile reads prog's "key=value" credentials file. See
// apiclient.ReadCredentialsFile.
func readCredentialsFile(prog string) (credentials, error) {
	return apiclient.ReadCredentialsFile(prog)
}

// writeCredentialsFile writes creds to prog's credentials file. See
// apiclient.WriteCredentialsFile.
func writeCredentialsFile(prog string, creds credentials) error {
	return apiclient.WriteCredentialsFile(prog, creds)
}
