package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAuthLoginCode redeems a dashboard pairing code over a cookie-free POST,
// so it also works against a plain-http control plane.
func runAuthLoginCode(prog, apiURLFlag, profileFlag, code, clientName string, reauth bool, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), of outputFlags, jsonOut bool) int {
	profile := resolveProfile(profileFlag, lookupEnv)
	apiURL := resolveAPIURL(apiURLFlag, lookupEnv, prog, profile)
	serverInfo, err := checkLoginServer(prog, profile, apiURL, reauth, stderr)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	if clientName == "" {
		if h, herr := os.Hostname(); herr == nil {
			clientName = h
		}
	}
	paired, err := apiclient.NewClient(apiURL, "").RedeemCLIPairing(context.Background(), apiclient.CLIPairRequest{Code: code, ClientName: clientName})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("pair with the pairing code (it is single-use and expires in minutes; copy a new one from the dashboard): %w", err))
	}
	if err := writeCredentialsFile(prog, profile, withServerPin(credentials{APIURL: apiURL, Token: paired.Token}, serverInfo)); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("save credentials: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, paired, func() {
		dir, _ := configDir(prog)
		_, _ = fmt.Fprintf(stdout, "logged in as %q\n", paired.Username)
		_, _ = fmt.Fprintf(stdout, "token %s saved to %s/%s under profile %q, expires %s\n", paired.TokenID, dir, credentialsFileName, profile, paired.ExpiresAt.Local().Format("2006-01-02"))
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
