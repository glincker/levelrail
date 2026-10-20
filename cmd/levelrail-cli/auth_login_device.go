package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

// Defensive floor so a malformed server response cannot spin the poll loop.
const defaultDevicePollInterval = 5 * time.Second

// Cookie-free JSON POSTs, so it works against a plain-http control plane.
func runAuthLoginDevice(prog, apiURLFlag, profileFlag, clientNameFlag string, reauth bool, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), of outputFlags, jsonOut bool) int {
	profile := resolveProfile(profileFlag, lookupEnv)
	apiURL := resolveAPIURL(apiURLFlag, lookupEnv, prog, profile)
	client := NewClient(apiURL, "")
	serverInfo, err := checkLoginServer(prog, profile, apiURL, reauth, stderr)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

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

	approvalURL := started.VerificationURIComplete
	if approvalURL == "" {
		approvalURL = started.VerificationURI
	}
	expiresAt := time.Now().UTC().Add(time.Duration(started.ExpiresIn) * time.Second)
	if jsonOut || of.Format == outputJSON {
		if err := writeDevicePendingLine(stdout, approvalURL, started, expiresAt); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("write pending line: %w", err))
		}
	}
	_, _ = fmt.Fprintf(stderr, "To finish logging in, open:\n\n  %s\n\nand confirm this code matches: %s\nThe approval page must show the same server fingerprint as printed above.\n\nThe code expires in %d minutes. Waiting for approval...\n", approvalURL, started.UserCode, (started.ExpiresIn+59)/60)

	interval := time.Duration(started.Interval) * time.Second
	if interval <= 0 {
		interval = defaultDevicePollInterval
	}
	timeout := time.Duration(started.ExpiresIn) * time.Second

	granted, err := pollDeviceAuthUntilGranted(ctx, client, started.DeviceCode, interval, timeout, time.Sleep)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	if err := writeCredentialsFile(prog, profile, withServerPin(credentials{APIURL: apiURL, Token: granted.Token}, serverInfo)); err != nil {
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

// sleep is injected so tests can drive the loop without a real wait.
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

// devicePendingLine is the single machine-readable line "--json" prints
// before blocking, so an agent can relay the approval link to its user.
type devicePendingLine struct {
	Event           string    `json:"event"`
	VerificationURL string    `json:"verification_url"`
	UserCode        string    `json:"user_code"`
	ExpiresIn       int       `json:"expires_in"`
	ExpiresAt       time.Time `json:"expires_at"`
}

func writeDevicePendingLine(w io.Writer, approvalURL string, started deviceStartResponse, expiresAt time.Time) error {
	b, err := json.Marshal(devicePendingLine{
		Event: "device_login_pending", VerificationURL: approvalURL, UserCode: started.UserCode,
		ExpiresIn: started.ExpiresIn, ExpiresAt: expiresAt,
	})
	if err != nil {
		return fmt.Errorf("encode pending line: %w", err)
	}
	_, err = fmt.Fprintf(w, "%s\n", b)
	return err
}
