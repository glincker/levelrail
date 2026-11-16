package apiclient

import (
	"fmt"
	"net/http"
	"strings"
)

// URLOrigin names where a resolved API address came from.
type URLOrigin string

// Address origins, as reported by ResolveAPIURLOrigin.
const (
	OriginFlag         URLOrigin = "flag"
	OriginEnv          URLOrigin = "env"
	OriginSavedPointer URLOrigin = "repo pointer matching a saved profile"
	OriginSaved        URLOrigin = "profile"
	OriginDefault      URLOrigin = "default"
)

// ResolvedURL is an API address plus where it came from. URL is never an address
// that only the repo pointer supplied; credentials reach an address only through ForCredentials.
type ResolvedURL struct {
	URL    string
	Origin URLOrigin

	refusal      *PointerMismatch
	matchProfile string
	matchPin     ServerPin
}

// CredentialURL is an address a token or password may be sent to: typed, set in the environment, or saved.
type CredentialURL struct{ url string }

func (c CredentialURL) String() string { return c.url }

// ResolveAPIURLOrigin resolves the address by precedence: flag, env, a repo pointer
// address that a saved profile already uses, the profile's saved address, then the default.
func ResolveAPIURLOrigin(flagURL string, lookupEnv func(string) (string, bool), prog, profile string) ResolvedURL {
	if flagURL != "" {
		return ResolvedURL{URL: flagURL, Origin: OriginFlag}
	}
	if v, ok := lookupEnv(EnvAPIURL); ok && v != "" {
		return ResolvedURL{URL: v, Origin: OriginEnv}
	}
	fallback := ResolvedURL{URL: DefaultAPIURL, Origin: OriginDefault}
	creds, credsErr := ReadCredentialsFile(prog, profile)
	if credsErr == nil && creds.APIURL != "" {
		fallback = ResolvedURL{URL: creds.APIURL, Origin: OriginSaved}
	}
	p, ok := FindRepoPointerFromCwd()
	// The pointer only applies to the profile it names, so another profile keeps its own address.
	if !ok || (p.Profile != "" && p.Profile != profile) {
		return fallback
	}
	if p.Problem != "" {
		fallback.refusal = &PointerMismatch{Profile: profile, PointerURL: p.RejectedURL, Path: p.Path, Reason: p.Problem}
		return fallback
	}
	if p.APIURL == "" {
		return fallback
	}
	if name, pin, ok := savedProfileFor(p.APIURL, prog, profile, creds); ok {
		return ResolvedURL{URL: p.APIURL, Origin: OriginSavedPointer, matchProfile: name, matchPin: pin}
	}
	fallback.refusal = &PointerMismatch{Profile: profile, PointerURL: p.APIURL, SavedURL: creds.APIURL, Path: p.Path}
	return fallback
}

// savedProfileFor finds the saved profile whose address equals pointerURL. A profile
// with its own saved address only accepts that address, so its login never moves.
func savedProfileFor(pointerURL, prog, profile string, active Credentials) (string, ServerPin, bool) {
	if active.APIURL != "" {
		return profile, active.pin(), sameURL(active.APIURL, pointerURL)
	}
	summaries, err := ListProfiles(prog)
	if err != nil {
		return "", ServerPin{}, false
	}
	for _, s := range summaries {
		if s.APIURL == "" || !sameURL(s.APIURL, pointerURL) {
			continue
		}
		c, err := ReadCredentialsFile(prog, s.Name)
		if err != nil {
			return "", ServerPin{}, false
		}
		return s.Name, c.pin(), true
	}
	return "", ServerPin{}, false
}

func sameURL(a, b string) bool { return strings.TrimRight(a, "/") == strings.TrimRight(b, "/") }

// Refusal reports the pointer address that was set aside, if any.
func (r ResolvedURL) Refusal() (PointerMismatch, bool) {
	if r.refusal == nil {
		return PointerMismatch{}, false
	}
	return *r.refusal, true
}

// ForCredentials is the only way to an address a credential may go to. A pointer address
// matching a pinned profile is held to that profile's pin for token, whatever its source.
func (r ResolvedURL) ForCredentials(token, prog string) (CredentialURL, error) {
	if r.refusal != nil {
		m := *r.refusal
		m.Prog = prog
		return CredentialURL{}, m
	}
	if r.Origin == OriginSavedPointer && token != "" && r.matchPin.InstanceID != "" {
		registerTokenPin(token, tokenPin{pin: r.matchPin, enforce: true, prog: prog, profile: r.matchProfile})
	}
	return CredentialURL{url: r.URL}, nil
}

// PointerMismatch describes a repo pointer address no saved profile uses, or one the pointer file may not name.
type PointerMismatch struct {
	Prog, Profile, PointerURL, SavedURL, Path, Reason string
}

// FindPointerMismatch reports a pointer address that was set aside for this profile.
// An explicit flag or env URL is the user's own choice and never counts.
func FindPointerMismatch(flagURL string, lookupEnv func(string) (string, bool), prog, profile string) (PointerMismatch, bool) {
	return ResolveAPIURLOrigin(flagURL, lookupEnv, prog, profile).Refusal()
}

// NextCommand is the deliberate command that makes the pointer address a saved one.
func (m PointerMismatch) NextCommand() string {
	prog := m.Prog
	if prog == "" {
		prog = "levelrail-cli"
	}
	if m.Reason != "" {
		return fmt.Sprintf("edit %s so api_url is an https address, or pass --api-url to %s", m.Path, prog)
	}
	if m.SavedURL != "" {
		return fmt.Sprintf("%s auth login --reauth --profile %s --api-url %s", prog, m.Profile, m.PointerURL)
	}
	return fmt.Sprintf("%s auth login --profile %s --api-url %s", prog, m.Profile, m.PointerURL)
}

// Error names the addresses involved and both deliberate ways forward.
func (m PointerMismatch) Error() string {
	const rule = "credentials are only sent to an address you saved or typed"
	if m.Reason != "" {
		return fmt.Sprintf("the repo pointer %s is not used: %s. Pass --api-url to choose the address, or set api_url in the pointer to an https address", m.Path, m.Reason)
	}
	if m.SavedURL != "" {
		return fmt.Sprintf("the repo pointer address %s differs from the address saved for profile %q (%s); %s. Pass --api-url %s to use it for one command, or save it on the profile with: %s",
			m.PointerURL, m.Profile, m.SavedURL, rule, m.PointerURL, m.NextCommand())
	}
	return fmt.Sprintf("the repo pointer address %s (from %s) is not saved in any profile; %s. Pass --api-url %s to use it for one command, or save it with: %s",
		m.PointerURL, m.Path, rule, m.PointerURL, m.NextCommand())
}

// refusingTransport fails every request before it reaches the network.
type refusingTransport struct{ err error }

func (t refusingTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, t.err }

// RefusingTransport returns a transport that sends nothing and always returns err.
func RefusingTransport(err error) http.RoundTripper { return refusingTransport{err: err} }
