package runtimesettings

import "fmt"

// Keys for how a bare domain and its www twin are set up.
const (
	KeyDomainsWWWMode   = "domains.www_mode"
	KeyDomainsCanonical = "domains.canonical"
)

func oneOfCheck(allowed ...string) func(string) error {
	return func(v string) error {
		for _, a := range allowed {
			if v == a {
				return nil
			}
		}
		return fmt.Errorf("must be one of %v", allowed)
	}
}

func init() {
	Register(Definition{
		Key: KeyDomainsWWWMode, Category: CategoryNetworking, Kind: KindString, Default: "both",
		Env: "APP_DOMAINS_WWW_MODE", Apply: ApplyLive, CheckValue: oneOfCheck("both", "apex_only", "www_only"),
		Title:       "Domains: apex and www records",
		Description: "Which hosts get DNS records when a bare domain is planned: both (www follows the apex), apex_only or www_only. The next plan uses a changed value.",
	})
	Register(Definition{
		Key: KeyDomainsCanonical, Category: CategoryNetworking, Kind: KindString, Default: "apex",
		Env: "APP_DOMAINS_CANONICAL", Apply: ApplyLive, CheckValue: oneOfCheck("apex", "www"),
		Title:       "Domains: canonical host",
		Description: "Which host visitors end on when both are served: apex or www. The other host answers with a 308 redirect. The next plan uses a changed value.",
	})
}
