package main

import (
	"context"
	"fmt"
	"io"
)

func runPipelinesOIDC(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) > 0 && args[0] == "rotate-key" {
		return runPipelinesOIDCRotateKey(prog, args[1:], stdout, stderr, lookupEnv)
	}
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
		_, _ = fmt.Fprintf(stdout, "Issuer URL: %s\nJWKS URL:   %s\nKey rotation supported: %t\n", info.IssuerURL, info.JWKSURL, info.RotationSupported)
	})
}

// runPipelinesOIDCRotateKey handles `pipelines oidc rotate-key`: rotates
// the pipeline OIDC signing key immediately. The previous key stays
// published in the JWKS (see docs/pipelines-oidc.md) until --retire-after
// elapses, so tokens it already signed keep verifying.
func runPipelinesOIDCRotateKey(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newPipelineCmd(prog, "pipelines oidc rotate-key", "print the rotation result as JSON", stdout, stderr)
	retireAfter := c.fs.String("retire-after", "", "how long the previous signing key stays published in the JWKS (Go duration, e.g. 1h); default 24h")
	client, _, of, jsonOut, code, ok := c.parse(args, 0, 0, lookupEnv)
	if !ok {
		return code
	}
	res, err := client.RotatePipelineOIDCKey(context.Background(), *retireAfter)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("rotate pipeline oidc key: %w", err))
	}
	return c.write(of, res, func() {
		_, _ = fmt.Fprintf(stdout, "Rotated signing key %s -> %s.\n", res.OldKID, res.NewKID)
		_, _ = fmt.Fprintf(stdout, "WARNING: the previous key (%s) stays published in the JWKS until %s. Tokens it signed keep verifying until then; do not remove it from any external cache sooner.\n",
			res.OldKID, res.RetireAt.Format("2006-01-02T15:04:05Z07:00"))
	})
}
