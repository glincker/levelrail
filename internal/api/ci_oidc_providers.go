package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	ciProviderGitHub   = "github"
	ciProviderGitLab   = "gitlab"
	ciProviderCircleCI = "circleci"

	gitLabDefaultIssuer = "https://gitlab.com"
	gitLabJWKSPath      = "/oauth/discovery/keys"
	circleCIIssuerBase  = "https://oidc.circleci.com/org/"
	circleCIJWKSPath    = "/.well-known/jwks-pub.json"

	circleCIClaimProjectID  = "oidc.circleci.com/project-id"
	circleCIClaimContextIDs = "oidc.circleci.com/context-ids"
	circleCIClaimVCSRef     = "oidc.circleci.com/vcs-ref"
)

// ciProviderSpecFor derives the JWKS URL from the trusted issuer, never from anything in the token.
func ciProviderSpecFor(provider, issuer string) (ciProviderSpec, error) {
	switch provider {
	case ciProviderGitLab:
		return ciProviderSpec{Provider: provider, Issuer: issuer, JWKSURL: issuer + gitLabJWKSPath}, nil
	case ciProviderCircleCI:
		return ciProviderSpec{Provider: provider, Issuer: issuer, JWKSURL: issuer + circleCIJWKSPath}, nil
	}
	return ciProviderSpec{}, fmt.Errorf("api: no oidc verifier for ci provider %q", provider)
}

func mapCIClaims(spec ciProviderSpec, subject string, all map[string]json.RawMessage) (CIClaims, error) {
	switch spec.Provider {
	case ciProviderGitLab:
		return gitLabCIClaims(spec, subject, all)
	case ciProviderCircleCI:
		return circleCIClaims(spec, subject, all)
	}
	return CIClaims{}, fmt.Errorf("api: no claim mapping for ci provider %q", spec.Provider)
}

// gitLabCIClaims turns ref plus ref_type into a full ref so trusts use the same refs/heads/* patterns as GitHub.
func gitLabCIClaims(spec ciProviderSpec, subject string, all map[string]json.RawMessage) (CIClaims, error) {
	project, ref := claimString(all, "project_path"), claimString(all, "ref")
	if project == "" || ref == "" {
		return CIClaims{}, errors.New("api: gitlab id token has no project_path or ref claim")
	}
	if ns := claimString(all, "namespace_path"); ns != "" && !strings.HasPrefix(project, ns+"/") {
		return CIClaims{}, errors.New("api: gitlab id token project_path is outside its namespace_path")
	}
	var full string
	switch claimString(all, "ref_type") {
	case "branch":
		full = "refs/heads/" + ref
	case "tag":
		full = "refs/tags/" + ref
	default:
		return CIClaims{}, errors.New("api: gitlab id token has no branch or tag ref_type")
	}
	return CIClaims{
		Provider: ciProviderGitLab, Issuer: spec.Issuer, Repository: project, Ref: full,
		Environment: claimString(all, "environment"), RefProtected: claimBool(all, "ref_protected"), Subject: subject,
	}, nil
}

// circleCIClaims cross-checks sub against the issuer's org and the project-id claim before trusting either.
func circleCIClaims(spec ciProviderSpec, subject string, all map[string]json.RawMessage) (CIClaims, error) {
	project := claimString(all, circleCIClaimProjectID)
	parts := strings.Split(subject, "/")
	if project == "" || len(parts) < 4 || parts[0] != "org" || parts[2] != "project" || parts[3] != project {
		return CIClaims{}, errors.New("api: circleci oidc token subject does not name its project")
	}
	if !strings.HasSuffix(spec.Issuer, "/org/"+parts[1]) {
		return CIClaims{}, errors.New("api: circleci oidc token subject is from another organization")
	}
	var contexts []string
	if raw, ok := all[circleCIClaimContextIDs]; ok {
		if err := json.Unmarshal(raw, &contexts); err != nil {
			return CIClaims{}, errors.New("api: circleci oidc token context-ids is not a list")
		}
	}
	return CIClaims{
		Provider: ciProviderCircleCI, Issuer: spec.Issuer, Repository: project,
		Ref: claimString(all, circleCIClaimVCSRef), ContextIDs: contexts, Subject: subject,
	}, nil
}

func claimString(all map[string]json.RawMessage, name string) string {
	var s string
	if raw, ok := all[name]; ok && json.Unmarshal(raw, &s) == nil {
		return s
	}
	return ""
}

// claimBool accepts GitLab's string "true" as well as a JSON boolean.
func claimBool(all map[string]json.RawMessage, name string) bool {
	raw, ok := all[name]
	if !ok {
		return false
	}
	var b bool
	if json.Unmarshal(raw, &b) == nil {
		return b
	}
	return claimString(all, name) == "true"
}
