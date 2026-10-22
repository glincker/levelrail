package main

import (
	"fmt"
	"io"
	"strings"

	"context"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const databasesCredentialsLabel = "databases credentials"

func runDatabasesCredentials(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	usage := "databases credentials <database> [flags]\n\n" +
		"Prints the generated owner password of a managed database. Owner only: it asks\n" +
		"for your password (and authenticator code), is rate limited and is audited."
	c, code, ok := parseDBAccess(prog, databasesCredentialsLabel, usage, args, 1, "a database name", stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	name := strings.TrimSpace(c.rest[0])
	res, err := withStepUp(stderr, func(p *apiclient.StepUpProof) (apiclient.DatabaseRevealedCredential, error) {
		return c.client.RevealDatabaseCredentials(context.Background(), name, p)
	})
	if err != nil {
		return c.fail(fmt.Errorf("reveal credentials of database %q: %w", name, err))
	}
	return c.result(res, func() { _, _ = fmt.Fprintln(stdout, res.Password) })
}
