package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

// defaultDevicePollInterval is the fallback poll interval when the
// server's own DeviceStartResponse.Interval is somehow non-positive:
// never expected in practice (internal/api's devicePollInterval is a
// fixed positive constant), just a defensive floor so a malformed
// response can't spin this loop.
const defaultDevicePollInterval = 5 * time.Second

// runAuthLoginDevice implements "auth login --device": the terminal
// half of a `gh auth login`-style device code flow. POST
// /api/v1/auth/device/start (unauthenticated) mints a device_code this
// process polls and a short user_code it prints for the operator to
// type into the web dashboard's CLI Access page; once approved there,
// the next poll returns a real API token, saved to this CLI's own
// credentials file exactly like "auth login"'s username/password path
// does.
//
// Unlike the username/password path (see runAuthLogin's own doc
// comment on the plain-http gap that path inherits from needing a
// Secure session cookie to round-trip), every request this flow makes
// is a plain, cookie-free JSON POST, so it works against a local,
// plain-http control plane (the common http://localhost:8080 dev
// target) with no caveat.
func runAuthLoginDevice(prog, apiURLFlag, profileFlag, clientNameFlag string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), of outputFlags, jsonOut bool) int {
	profile := resolveProfile(profileFlag, lookupEnv)
	apiURL := resolveAPIURL(apiURLFlag, lookupEnv, prog, profile)
	client := NewClient(apiURL, "")

	clientName := clientNameFlag
	if clientName == "" {
		if h, err := os.Hostname(); err == nil {
			clientName = h
		}
	}

	ctx := context.Background()
	started, err := client.StartDeviceAuth(ctx, deviceStartRequest{ClientName: clientName})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("start device login: %w", err))
	}

	_, _ = fmt.Fprintf(stderr, "To finish logging in, open:\n\n  %s\n\nand enter this code: %s\n\nWaiting for approval...\n", started.VerificationURI, started.UserCode)

	interval := time.Duration(started.Interval) * time.Second
	if interval <= 0 {
		interval = defaultDevicePollInterval
	}
	timeout := time.Duration(started.ExpiresIn) * time.Second

	granted, err := pollDeviceAuthUntilGranted(ctx, client, started.DeviceCode, interval, timeout, time.Sleep)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	if err := writeCredentialsFile(prog, profile, credentials{APIURL: apiURL, Token: granted.Token}); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("save credentials: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, granted, func() {
		dir, _ := configDir(prog)
		_, _ = fmt.Fprintln(stdout, "device login approved")
		_, _ = fmt.Fprintf(stdout, "token %q (id %s) created and saved to %s/%s under profile %q\n", granted.Name, granted.ID, dir, credentialsFileName, profile)
		_, _ = fmt.Fprintf(stdout, "token value (shown once, not recoverable again): %s\n", granted.Token)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// pollDeviceAuthUntilGranted polls PollDeviceAuthToken every interval
// until it returns a token, a terminal denial/expiry, or timeout
// elapses. sleep is injected (real callers pass time.Sleep) so this
// package's own tests can drive the loop without a real wait.
func pollDeviceAuthUntilGranted(ctx context.Context, client *Client, deviceCode string, interval, timeout time.Duration, sleep func(time.Duration)) (deviceTokenResponse, error) {
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return deviceTokenResponse{}, errors.New(`device login code expired, run "auth login --device" again`)
		}
		sleep(interval)

		granted, err := client.PollDeviceAuthToken(ctx, deviceCode)
		if err == nil {
			return granted, nil
		}

		var apiErr *apiError
		if errors.As(err, &apiErr) {
			switch apiErr.Message {
			case "authorization_pending":
				continue
			case "access_denied":
				return deviceTokenResponse{}, errors.New("device login was denied")
			case "expired_token":
				return deviceTokenResponse{}, errors.New(`device login code expired, run "auth login --device" again`)
			}
		}
		return deviceTokenResponse{}, fmt.Errorf("poll device login: %w", err)
	}
}
