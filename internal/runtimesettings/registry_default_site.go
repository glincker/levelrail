package runtimesettings

import (
	"fmt"
	"net/url"
)

// Registered keys for what the ingress answers when no route claims the requested host.
const (
	KeyIngressUnmatchedMode        = "ingress.unmatched_mode"
	KeyIngressUnmatchedRedirectURL = "ingress.unmatched_redirect_url"
	// UnmatchedDiagnostic, UnmatchedNotFound and UnmatchedRedirect are the accepted ingress.unmatched_mode values.
	UnmatchedDiagnostic = "diagnostic"
	UnmatchedNotFound   = "not_found"
	UnmatchedRedirect   = "redirect"
)

func init() {
	Register(Definition{
		Key: KeyIngressUnmatchedMode, Category: CategoryNetworking, Kind: KindString, Default: UnmatchedDiagnostic,
		Env: "APP_INGRESS_UNMATCHED_MODE", Apply: ApplyLive, AllowedStrings: []string{UnmatchedDiagnostic, UnmatchedNotFound, UnmatchedRedirect},
		Title:       "Unmatched host behavior",
		Description: "What the ingress answers for a host no app claims, including the bare server address. diagnostic shows the help page, not_found answers a plain 404 with no detail, redirect sends visitors to the URL in ingress.unmatched_redirect_url. A saved change re-renders the ingress config at once, with no restart.",
	})
	Register(Definition{
		Key: KeyIngressUnmatchedRedirectURL, Category: CategoryNetworking, Kind: KindString, Default: "",
		Env: "APP_INGRESS_UNMATCHED_REDIRECT_URL", Apply: ApplyLive, CheckValue: checkUnmatchedRedirectURL,
		Title:       "Unmatched host redirect target",
		Description: "HTTP or HTTPS URL that unmatched hosts are redirected to when ingress.unmatched_mode is redirect. Empty with redirect mode falls back to the diagnostic page. A saved change re-renders the ingress config at once.",
	})
}

// checkUnmatchedRedirectURL accepts empty or an absolute http(s) URL so the Location header is never a script or relative target.
func checkUnmatchedRedirectURL(v string) error {
	if v == "" {
		return nil
	}
	u, err := url.Parse(v)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("must be an absolute http or https URL")
	}
	return nil
}
