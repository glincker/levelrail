package main

import (
	"context"
	"fmt"
	"io"
)

func runPipelinesOIDC(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newPipelineCmd(prog, "pipelines oidc", "print OIDC federation status as JSON", stdout, stderr)
	client, _, of, jsonOut, code, ok := c.parse(args, 0, 0, lookupEnv)
	if !ok {
		return code
	}
	info, err := client.GetPipelineOIDCInfo(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get pipeline oidc info: %w", err))
	}
	return c.write(of, info, func() {
		if !info.Configured {
			_, _ = fmt.Fprintln(stdout, "OIDC federation is not configured on this control plane. Set APP_OIDC_ISSUER_URL to a reachable HTTPS URL to enable it.")
			return
		}
		_, _ = fmt.Fprintf(stdout, "Issuer URL: %s\nJWKS URL:   %s\n", info.IssuerURL, info.JWKSURL)
	})
}
