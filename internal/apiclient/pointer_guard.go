package apiclient

import (
	"fmt"
	"net/http"
	"strings"
)

// PointerMismatch describes a repo pointer address that differs from the address a profile's saved login belongs to.
type PointerMismatch struct {
	Profile, PointerURL, SavedURL string
}

// FindPointerMismatch reports a pointer api_url that differs from the saved
// address of a profile holding saved credentials. An explicit flag or env URL is the user's own choice and never counts.
func FindPointerMismatch(flagURL string, lookupEnv func(string) (string, bool), prog, profile string) (PointerMismatch, bool) {
	if flagURL != "" {
		return PointerMismatch{}, false
	}
	if v, ok := lookupEnv(EnvAPIURL); ok && v != "" {
		return PointerMismatch{}, false
	}
	p, ok := FindRepoPointerFromCwd()
	if !ok || p.APIURL == "" || (p.Profile != "" && p.Profile != profile) {
		return PointerMismatch{}, false
	}
	creds, err := ReadCredentialsFile(prog, profile)
	if err != nil || creds.Token == "" || creds.APIURL == "" {
		return PointerMismatch{}, false
	}
	if strings.TrimRight(p.APIURL, "/") == strings.TrimRight(creds.APIURL, "/") {
		return PointerMismatch{}, false
	}
	return PointerMismatch{Profile: profile, PointerURL: p.APIURL, SavedURL: creds.APIURL}, true
}

// Error names both addresses and the deliberate command that moves the profile.
func (m PointerMismatch) Error() string {
	return fmt.Sprintf("the repo pointer address %s differs from the address saved for profile %q (%s); a saved login is only sent to the address it was saved for. Edit the pointer to match, or sign in to the new address on purpose with: auth login --reauth --profile %s --api-url %s",
		m.PointerURL, m.Profile, m.SavedURL, m.Profile, m.PointerURL)
}

// refusingTransport fails every request before it reaches the network.
type refusingTransport struct{ err error }

func (t refusingTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, t.err }

// RefusingTransport returns a transport that sends nothing and always returns err.
func RefusingTransport(err error) http.RoundTripper { return refusingTransport{err: err} }
