package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const instancePath = "/api/v1/instance"

// InstanceInfo mirrors internal/api's instanceResource, the public server identity.
type InstanceInfo struct {
	InstanceID  string `json:"instance_id"`
	Name        string `json:"name"`
	ShortName   string `json:"short_name"`
	Version     string `json:"version"`
	Fingerprint string `json:"fingerprint"`
}

// ServerPin is the server a credentials profile was logged in to.
type ServerPin struct {
	InstanceID  string
	Fingerprint string
	Name        string
	APIURL      string
}

// ServerMismatchError is returned instead of sending a token to a server that is not the
// one the profile is pinned to.
type ServerMismatchError struct {
	Prog, Profile, URL string
	Pinned             ServerPin
	Got                *InstanceInfo
}

func (e *ServerMismatchError) Error() string {
	got := "no server identity (older server or not this product)"
	if e.Got != nil {
		got = fmt.Sprintf("%q (%s, fingerprint %s)", e.Got.Name, ShortInstanceID(e.Got.InstanceID), e.Got.Fingerprint)
	}
	return fmt.Sprintf("refusing to send the token for profile %q to %s: the profile is pinned to %q (%s, fingerprint %s) but this URL answered as %s. Run \"%s auth login --reauth --profile %s\" to log in to this server, or \"%s auth trust-server --profile %s\" to pin it instead",
		e.Profile, e.URL, e.Pinned.Name, ShortInstanceID(e.Pinned.InstanceID), e.Pinned.Fingerprint, got, e.Prog, e.Profile, e.Prog, e.Profile)
}

// ShortInstanceID abbreviates an instance id for display.
func ShortInstanceID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

// TrustRequest is what a first connect under an unpinned profile asks the operator to confirm.
type TrustRequest struct {
	Prog, Profile, URL string
	Info               InstanceInfo
}

type tokenPin struct {
	pin     ServerPin
	enforce bool
	prog    string
	profile string
}

var pinState = struct {
	mu       sync.Mutex
	byToken  map[string]tokenPin
	verified map[string]error
	trust    func(TrustRequest) error
	warn     io.Writer
}{byToken: map[string]tokenPin{}, verified: map[string]error{}}

func registerTokenPin(token string, p tokenPin) {
	pinState.mu.Lock()
	defer pinState.mu.Unlock()
	pinState.byToken[token] = p
}

// SetTrustHook installs the first-connect confirmation (trust on first use). With no hook an
// unpinned profile is allowed through, as it was before pinning existed.
func SetTrustHook(fn func(TrustRequest) error) {
	pinState.mu.Lock()
	pinState.trust = fn
	pinState.mu.Unlock()
}

// SetWarnWriter sets where non-fatal pin warnings go (nil discards them).
func SetWarnWriter(w io.Writer) {
	pinState.mu.Lock()
	pinState.warn = w
	pinState.mu.Unlock()
}

// ResetServerPins clears the per-process pin and verification state (tests).
func ResetServerPins() {
	pinState.mu.Lock()
	pinState.byToken = map[string]tokenPin{}
	pinState.verified = map[string]error{}
	pinState.mu.Unlock()
}

// FetchInstanceInfo reads GET /api/v1/instance. A nil info with a nil error means the server
// answered 404: it predates server identity.
func FetchInstanceInfo(ctx context.Context, hc *http.Client, baseURL string) (*InstanceInfo, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+instancePath, nil) //nolint:gosec // operator-supplied API target
	if err != nil {
		return nil, fmt.Errorf("build instance request: %w", err)
	}
	resp, err := hc.Do(req) //nolint:gosec // same target as above
	if err != nil {
		return nil, fmt.Errorf("read server identity from %s: %w", baseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("read server identity from %s: status %d", baseURL, resp.StatusCode)
	}
	var info InstanceInfo
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&info); err != nil || info.InstanceID == "" {
		return nil, nil
	}
	return &info, nil
}

// pinTransport checks the server identity once per token and origin before the first
// request that carries a pinned token, so a wrong URL, tunnel or hijacked DNS name never
// receives it.
type pinTransport struct{ base http.RoundTripper }

func (t pinTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	token := strings.TrimPrefix(req.Header.Get(headerAuthorization), bearerPrefix)
	if token != "" {
		pinState.mu.Lock()
		entry, ok := pinState.byToken[token]
		pinState.mu.Unlock()
		if ok {
			if err := t.verify(req, token, entry); err != nil {
				return nil, err
			}
		}
	}
	return t.base.RoundTrip(req)
}

func (t pinTransport) verify(req *http.Request, token string, e tokenPin) error {
	prefix := ""
	if i := strings.Index(req.URL.Path, "/api/v1"); i > 0 {
		prefix = req.URL.Path[:i]
	}
	origin := req.URL.Scheme + "://" + req.URL.Host + prefix
	key := origin + "|" + token

	pinState.mu.Lock()
	if err, done := pinState.verified[key]; done {
		pinState.mu.Unlock()
		return err
	}
	pinState.mu.Unlock()

	err := t.check(req.Context(), origin, e)
	pinState.mu.Lock()
	pinState.verified[key] = err
	pinState.mu.Unlock()
	return err
}

func (t pinTransport) check(ctx context.Context, origin string, e tokenPin) error {
	info, err := FetchInstanceInfo(ctx, &http.Client{Timeout: 10 * time.Second, Transport: t.base}, origin)
	if err != nil {
		return fmt.Errorf("cannot verify the server before sending a token: %w", err)
	}
	if e.pin.InstanceID == "" {
		return t.trustOnFirstUse(origin, e, info)
	}
	if info != nil && info.InstanceID == e.pin.InstanceID && info.Fingerprint == e.pin.Fingerprint {
		return nil
	}
	mismatch := &ServerMismatchError{Prog: e.prog, Profile: e.profile, URL: origin, Pinned: e.pin, Got: info}
	if e.enforce {
		return mismatch
	}
	pinState.mu.Lock()
	w := pinState.warn
	pinState.mu.Unlock()
	if w != nil {
		_, _ = fmt.Fprintf(w, "warning: %s\n", mismatch.Error())
	}
	return nil
}

func (t pinTransport) trustOnFirstUse(origin string, e tokenPin, info *InstanceInfo) error {
	if info == nil || !e.enforce {
		return nil
	}
	pinState.mu.Lock()
	hook := pinState.trust
	pinState.mu.Unlock()
	if hook == nil {
		return nil
	}
	if err := hook(TrustRequest{Prog: e.prog, Profile: e.profile, URL: origin, Info: *info}); err != nil {
		return errors.Join(errors.New("server not trusted, token not sent"), err)
	}
	return nil
}

// PinProfile records info as the server credentials is pinned to and saves it.
func PinProfile(prog, profile string, creds Credentials, info InstanceInfo) error {
	creds.InstanceID, creds.Fingerprint, creds.ServerName = info.InstanceID, info.Fingerprint, info.Name
	return WriteCredentialsFile(prog, profile, creds)
}
