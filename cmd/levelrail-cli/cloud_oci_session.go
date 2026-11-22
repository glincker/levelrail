package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ociSessionFiles is what the browser login leaves on this machine.
type ociSessionFiles struct {
	profileText, token, keyPEM string
}

// readOCIProfile reads one profile of ~/.oci/config plus its security_token_file and key_file
// locally. The server never reads this machine, so the contents travel in the request body.
func readOCIProfile(home, profile string) (ociSessionFiles, error) {
	if profile == "" {
		profile = "DEFAULT"
	}
	raw, err := os.ReadFile(filepath.Join(home, ".oci", "config")) //nolint:gosec // the user's own OCI config
	if err != nil {
		return ociSessionFiles{}, errors.New("could not read ~/.oci/config; run: oci session authenticate")
	}
	section, vals := "", map[string]string{}
	var kept []string
	for _, ln := range strings.Split(string(raw), "\n") {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			section = strings.Trim(t, "[]")
			continue
		}
		if k, v, ok := strings.Cut(t, "="); ok && section == profile {
			vals[strings.TrimSpace(k)] = strings.TrimSpace(v)
			if kk := strings.TrimSpace(k); kk == "tenancy" || kk == "region" {
				kept = append(kept, kk+"="+strings.TrimSpace(v))
			}
		}
	}
	tokenPath, keyPath := expandHome(home, vals["security_token_file"]), expandHome(home, vals["key_file"])
	if tokenPath == "" || keyPath == "" {
		return ociSessionFiles{}, fmt.Errorf("profile %s has no security_token_file and key_file; run: oci session authenticate --profile-name %s", profile, profile)
	}
	tok, err := os.ReadFile(tokenPath) //nolint:gosec // path named by the user's own profile
	if err != nil {
		return ociSessionFiles{}, errors.New("could not read the security token file; run: oci session authenticate")
	}
	key, err := os.ReadFile(keyPath) //nolint:gosec // path named by the user's own profile
	if err != nil {
		return ociSessionFiles{}, errors.New("could not read the session key file; run: oci session authenticate")
	}
	return ociSessionFiles{profileText: strings.Join(kept, "\n"), token: strings.TrimSpace(string(tok)), keyPEM: string(key)}, nil
}

func expandHome(home, p string) string {
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}

func reloginHint(profile string) string {
	if profile == "" {
		profile = "DEFAULT"
	}
	return "run: oci session authenticate --profile-name " + profile + ", then retry"
}
