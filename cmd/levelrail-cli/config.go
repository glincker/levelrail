package main

import "github.com/GLINCKER/levelrail/internal/apiclient"

// envAPIToken, envAPIURL, envProfile, defaultAPIURL, defaultProfile, and
// credentialsFileName alias internal/apiclient's exported constants of
// the same values: see client.go's own doc comment on why this file no
// longer defines its own credentials logic.
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
