package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const ciIDTokenTimeout = 30 * time.Second

func runAppsGitHubActionsWorkflow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps github-actions workflow", "print the workflow and its status as JSON to stdout and nothing else", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps github-actions workflow", "an app name", 1)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	wf, err := client.GetCIWorkflow(context.Background(), rest[0])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get ci workflow for %s: %w", rest[0], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, wf, func() {
		_, _ = fmt.Fprint(stdout, wf.YAML)
		if !wf.HasTrust {
			_, _ = fmt.Fprintf(stderr, "note: no CI trust yet, add one with: %s apps github-actions trust add %s --repo owner/name\n", prog, rest[0])
		}
	})
}

func runAppsGitHubActionsToken(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps github-actions token", "print the token as JSON to stdout and nothing else", stderr)
	var audience, idTokenEnv string
	fs.StringVar(&audience, "audience", "", "audience to request from GitHub (default: the API URL)")
	fs.StringVar(&idTokenEnv, "id-token-env", "", "read the identity token from this environment variable instead of GitHub (GitLab id_tokens, CircleCI OIDC)")
	_, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps github-actions token", "an app name", 1)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, "", profileFlag, lookupEnv)
	ctx, cancel := context.WithTimeout(context.Background(), ciIDTokenTimeout)
	defer cancel()
	var idToken string
	if idTokenEnv != "" {
		idToken, _ = lookupEnv(idTokenEnv)
		if idToken = strings.TrimSpace(idToken); idToken == "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("%s is empty: request an identity token for the control plane URL in the job first", idTokenEnv))
		}
	} else {
		reqURL, _ := lookupEnv("ACTIONS_ID_TOKEN_REQUEST_URL")
		reqToken, _ := lookupEnv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
		if reqURL == "" || reqToken == "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("no GitHub OIDC credentials: give the job permissions: id-token: write, or pass --id-token-env on another CI"))
		}
		if audience == "" {
			audience = strings.TrimRight(resolveAPIURL(apiURLFlag, lookupEnv, prog, resolveProfile(profileFlag, lookupEnv)), "/")
		}
		var err error
		if idToken, err = fetchGitHubIDToken(ctx, reqURL, reqToken, audience); err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
	}
	out, err := client.ExchangeCIToken(ctx, rest[0], idToken)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("exchange identity token for %s: %w", rest[0], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, out, func() { _, _ = fmt.Fprintln(stdout, out.Token) })
}

func fetchGitHubIDToken(ctx context.Context, reqURL, reqToken, audience string) (string, error) {
	u, err := url.Parse(reqURL)
	if err != nil {
		return "", fmt.Errorf("parse ACTIONS_ID_TOKEN_REQUEST_URL: %w", err)
	}
	q := u.Query()
	q.Set("audience", audience)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("build github oidc request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+reqToken)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req) //nolint:gosec // the URL is the runner-provided GitHub OIDC endpoint
	if err != nil {
		return "", fmt.Errorf("request github oidc token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github oidc token request answered %d", resp.StatusCode)
	}
	var body struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil || body.Value == "" {
		return "", fmt.Errorf("github oidc token response had no token")
	}
	return body.Value, nil
}

func runAppsGitHubActionsTrust(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(stderr, "Usage: %s apps github-actions trust list|add|remove <app> [flags]\n", prog)
		return exitUsage
	}
	switch args[0] {
	case "list":
		return runCITrustList(prog, args[1:], stdout, stderr, lookupEnv)
	case "add":
		return runCITrustAdd(prog, args[1:], stdout, stderr, lookupEnv)
	case "remove":
		return runCITrustRemove(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown trust subcommand %q (want list, add or remove)\n", prog, args[0])
		return exitUsage
	}
}

func runCITrustList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps github-actions trust list", "print the trusts as JSON to stdout and nothing else", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps github-actions trust list", "an app name", 1)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	trusts, err := client.ListCITrusts(context.Background(), rest[0])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list ci trusts for %s: %w", rest[0], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, trusts, func() { printCITrusts(stdout, trusts) })
}

func printCITrusts(stdout io.Writer, trusts []apiclient.CITrust) {
	if len(trusts) == 0 {
		_, _ = fmt.Fprintln(stdout, "no CI trusts")
		return
	}
	for _, t := range trusts {
		env := t.Environment
		if env == "" {
			env = "any"
		}
		provider := t.Provider
		if provider == "" {
			provider = "github"
		}
		protected := ""
		if t.ProtectedRefOnly {
			protected = "  protected refs only"
		}
		_, _ = fmt.Fprintf(stdout, "%s  %s  %s  %s  environment %s%s\n", t.ID, provider, t.Repository, t.RefPattern, env, protected)
	}
}

func runCITrustAdd(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps github-actions trust add", "print the new trust as JSON to stdout and nothing else", stderr)
	var repo, ref, environment, provider, issuer, orgID string
	var protectedRef bool
	fs.StringVar(&provider, "provider", "github", "CI provider: github, gitlab or circleci")
	fs.StringVar(&repo, "repo", "", "GitHub owner/name, GitLab project path, or CircleCI project ID (required)")
	fs.StringVar(&ref, "ref", "refs/heads/main", "ref glob allowed to deploy, e.g. refs/heads/main or refs/tags/v*")
	fs.StringVar(&environment, "environment", "", "environment the job must run in, or for circleci a context ID it must use (default any)")
	fs.StringVar(&issuer, "issuer", "", "gitlab: issuer URL of a self-managed instance (default https://gitlab.com)")
	fs.StringVar(&orgID, "org-id", "", "circleci: organization ID (required for circleci)")
	fs.BoolVar(&protectedRef, "protected-ref", false, "gitlab: only accept pipelines on protected branches or tags")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps github-actions trust add", "an app name", 1)
	if !ok {
		return exitUsage
	}
	if repo == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--repo is required"))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	t, err := client.CreateCITrust(context.Background(), rest[0], apiclient.CITrustRequest{
		Provider: provider, Issuer: issuer, OrgID: orgID, Repository: repo, RefPattern: ref, Environment: environment, ProtectedRefOnly: protectedRef,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("add ci trust for %s: %w", rest[0], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, t, func() {
		_, _ = fmt.Fprintf(stdout, "trusted %s %s to deploy %s (%s)\n", t.Repository, t.RefPattern, rest[0], t.ID)
	})
}

func runCITrustRemove(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps github-actions trust remove", "print the result as JSON to stdout and nothing else", stderr)
	var id string
	fs.StringVar(&id, "id", "", "trust id from trust list (required)")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps github-actions trust remove", "an app name", 1)
	if !ok {
		return exitUsage
	}
	if id == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--id is required"))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.DeleteCITrust(context.Background(), rest[0], id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("remove ci trust %s: %w", id, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]string{"removed": id}, func() {
		_, _ = fmt.Fprintf(stdout, "removed %s\n", id)
	})
}
