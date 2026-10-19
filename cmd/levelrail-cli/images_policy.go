package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func runImagesPolicy(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var ref string
	c, code, ok := parseImagesCall(prog, "images policy", args, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&ref, "ref", "", "also verify this registry reference (repo@sha256:...) against the key")
	})
	if !ok {
		return code
	}
	res, err := c.client.ValidateImageSignaturePolicy(bg(), apiclient.ImageSignaturePolicyRequest{Ref: ref})
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("validate signature policy: %w", err))
	}
	exit := exitOK
	if !res.OK {
		exit = exitCheckFailed
	}
	if rc := writeScheduledTaskResult(stdout, stderr, c.of, res, func() {
		_, _ = fmt.Fprintf(stdout, "mode: %s (requested %s), key loaded: %t\n", res.Mode, res.RequestedMode, res.KeyConfigured)
		if res.KeyFingerprint != "" {
			_, _ = fmt.Fprintf(stdout, "key fingerprint: %s\n", res.KeyFingerprint)
		}
		if res.Sample != nil {
			_, _ = fmt.Fprintf(stdout, "sample: %s: %s\n", res.Sample.Status, res.Sample.Message)
		}
		for _, is := range res.Issues {
			_, _ = fmt.Fprintf(stdout, "issue: %s\n", is.Message)
		}
		if len(res.Issues) == 0 {
			_, _ = fmt.Fprintln(stdout, "policy is valid")
		}
	}); rc != exitOK {
		return rc
	}
	return exit
}
