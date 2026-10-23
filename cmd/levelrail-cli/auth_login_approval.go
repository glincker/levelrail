package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	envLoginApprovalTimeout = "APP_LOGIN_APPROVAL_TIMEOUT"
	envLoginApprovalWait    = "APP_LOGIN_APPROVAL_WAIT"
	defaultApprovalTimeout  = 3 * time.Minute
	approvalApprovePath     = "/approve-sign-in"
	approvalCountdownEvery  = 30 * time.Second
	approvalBackoffMax      = 10 * time.Second
)

// approvalPollInterval is the first wait between polls; it backs off from here.
var approvalPollInterval = 2 * time.Second

// openBrowserFunc is swapped by tests so none opens a real browser.
var openBrowserFunc = openBrowser

type approvalPollResponse struct {
	Status      string `json:"status"`
	Username    string `json:"username"`
	MFARequired bool   `json:"mfa_required"`
}

type approvalOptions struct {
	timeout time.Duration
	open    string
	wait    bool
}

type approvalOptionsKey struct{}

func withApprovalOptions(ctx context.Context, o approvalOptions) context.Context {
	return context.WithValue(ctx, approvalOptionsKey{}, o)
}

// triBool is a --open flag that also works bare: unset means "decide for me".
type triBool struct{ val string }

func (t *triBool) String() string   { return t.val }
func (t *triBool) IsBoolFlag() bool { return true }
func (t *triBool) Set(s string) error {
	b, err := strconv.ParseBool(s)
	if err != nil {
		return fmt.Errorf("want true or false: %w", err)
	}
	t.val = strconv.FormatBool(b)
	return nil
}

var _ flag.Value = (*triBool)(nil)

func approvalTimeoutFrom(flagVal time.Duration, lookupEnv func(string) (string, bool)) time.Duration {
	if flagVal > 0 {
		return flagVal
	}
	if raw, ok := lookupEnv(envLoginApprovalTimeout); ok {
		if d, err := time.ParseDuration(strings.TrimSpace(raw)); err == nil && d > 0 {
			return d
		}
	}
	return defaultApprovalTimeout
}

func envTruthy(lookupEnv func(string) (string, bool), key string) bool {
	v, ok := lookupEnv(key)
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	return ok && err == nil && b
}

// canOpenBrowser is false over SSH and on a Linux box with no display.
func canOpenBrowser(lookupEnv func(string) (string, bool)) bool {
	for _, k := range []string{"SSH_CONNECTION", "SSH_TTY", "SSH_CLIENT"} {
		if v, ok := lookupEnv(k); ok && v != "" {
			return false
		}
	}
	if runtime.GOOS == "linux" {
		for _, k := range []string{"DISPLAY", "WAYLAND_DISPLAY"} {
			if v, ok := lookupEnv(k); ok && v != "" {
				return true
			}
		}
		return false
	}
	return true
}

// approvalInteractive reports whether a person is at the terminal.
var approvalInteractive = func() bool {
	st, err := os.Stderr.Stat()
	return stdinIsTerminal() && err == nil && st.Mode()&os.ModeCharDevice != 0
}

func approvalPageURL(base, id string) string {
	return strings.TrimRight(base, "/") + approvalApprovePath + "?request=" + url.QueryEscape(id)
}

func approvalPairingHint(prog string) string {
	return fmt.Sprintf("Faster: open the dashboard, copy the command from the \"Connect your CLI\" card and run it (%s login --api-url <url> --code XXXX-XXXX).", prog)
}

// awaitApproval waits for another session of the account to approve this
// new sign-in; the poll carries the approval cookie the login response set.
func (c *authSessionClient) awaitApproval(ctx context.Context, prog string, login loginResponse, lookupEnv func(string) (string, bool), stderr io.Writer) (loginResponse, error) {
	opts, _ := ctx.Value(approvalOptionsKey{}).(approvalOptions)
	timeout := approvalTimeoutFrom(opts.timeout, lookupEnv)
	pageURL := approvalPageURL(c.baseURL, login.ApprovalID)
	interactive := approvalInteractive()
	if !interactive && !opts.wait && !envTruthy(lookupEnv, envLoginApprovalWait) {
		return loginResponse{}, &apiError{StatusCode: http.StatusUnauthorized, Message: fmt.Sprintf(
			"new-device approval required (request %s, number %d): approve at %s, or sign in with \"%s login --api-url %s --code XXXX-XXXX\" using a code from the dashboard, or re-run with --wait to wait for approval",
			login.ApprovalID, login.ApprovalMatch, pageURL, prog, c.baseURL)}
	}
	if login.ApprovalExpiresAt != nil {
		if left := time.Until(*login.ApprovalExpiresAt); left > 0 && left < timeout {
			timeout = left
		}
	}
	_, _ = fmt.Fprintf(stderr, "This sign-in comes from a new device, so it needs your approval.\n\n  1. Open %s\n     in the browser where you are signed in to the dashboard.\n  2. Type this number when asked: %d\n\nOr run \"%s auth approve %s --match %d\" with a token that holds signin:approve.\n%s\nWaiting up to %s. Press Ctrl+C to cancel.\n",
		pageURL, login.ApprovalMatch, prog, login.ApprovalID, login.ApprovalMatch, approvalPairingHint(prog), timeout.Round(time.Second))
	if wantOpen(opts.open, interactive, lookupEnv) {
		if err := openBrowserFunc(ctx, pageURL); err != nil {
			_, _ = fmt.Fprintf(stderr, "Could not open a browser (%v); open the link above yourself.\n", err)
		}
	}
	return c.pollApproval(ctx, prog, timeout, stderr)
}

func wantOpen(mode string, interactive bool, lookupEnv func(string) (string, bool)) bool {
	switch mode {
	case "true":
		return true
	case "false":
		return false
	}
	return interactive && canOpenBrowser(lookupEnv)
}

func (c *authSessionClient) pollApproval(ctx context.Context, prog string, timeout time.Duration, stderr io.Writer) (loginResponse, error) {
	deadline := time.Now().Add(timeout)
	nextNote := time.Now().Add(approvalCountdownEvery)
	wait := approvalPollInterval
	for {
		select {
		case <-ctx.Done():
			return loginResponse{}, ctx.Err()
		case <-time.After(wait):
		}
		wait = min(wait*3/2, approvalBackoffMax)
		var poll approvalPollResponse
		if err := c.do(ctx, http.MethodPost, "/api/v1/auth/login-approval/poll", struct{}{}, &poll); err != nil {
			return loginResponse{}, err
		}
		switch poll.Status {
		case "pending":
			now := time.Now()
			if !now.Before(deadline) {
				return loginResponse{}, &apiError{StatusCode: http.StatusUnauthorized, Message: fmt.Sprintf("nobody approved the sign-in within %s: run %s login again, or use a code from the dashboard's \"Connect your CLI\" card (--code)", timeout.Round(time.Second), prog)}
			}
			if !now.Before(nextNote) {
				_, _ = fmt.Fprintf(stderr, "Still waiting, %s left.\n", time.Until(deadline).Round(time.Second))
				nextNote = now.Add(approvalCountdownEvery)
			}
		case "approved":
			if poll.MFARequired {
				return loginResponse{}, &apiError{StatusCode: http.StatusUnauthorized, Message: "approved, but two-factor authentication is on: use " + prog + " auth login --device"}
			}
			return loginResponse{Username: poll.Username}, nil
		case "denied":
			return loginResponse{}, &apiError{StatusCode: http.StatusUnauthorized, Message: "the sign-in was denied from another session"}
		default:
			return loginResponse{}, &apiError{StatusCode: http.StatusUnauthorized, Message: "the sign-in request expired before anyone approved it: run " + prog + " login again"}
		}
	}
}
