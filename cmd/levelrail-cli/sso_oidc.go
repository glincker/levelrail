package main

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func (c cliCtx) printOIDCPolicies(ps ...apiclient.OAuthSSOPolicy) func() {
	return func() {
		tw := tabwriter.NewWriter(c.stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "PROVIDER\tENABLED\tJIT\tSYNC ROLES\tDEFAULT ROLE\tGROUPS CLAIM\tDOMAINS")
		for _, p := range ps {
			_, _ = fmt.Fprintf(tw, "%s\t%t\t%t\t%t\t%s\t%s\t%s\n", p.Provider, p.Enabled, p.JITProvisioning, p.SyncRoles,
				p.DefaultRoleID, p.GroupsClaim, strings.Join(p.AllowedDomains, ","))
		}
		_ = tw.Flush()
	}
}

func (c cliCtx) printOIDCEvaluation(e apiclient.OAuthSSOEvaluation) func() {
	return func() {
		verdict := "allowed"
		if !e.Allowed {
			verdict = "refused (" + e.RefusalCode + ")"
		}
		_, _ = fmt.Fprintf(c.stdout, "result:   %s\nrole:     %s (%s)\naccount:  %s\ngroups:   %s\n", verdict, e.RoleID, e.RoleSource,
			e.AccountState, strings.Join(e.Groups, ", "))
		for _, i := range e.Issues {
			hint := i.Fix.Kind
			if i.Fix.Action != "" {
				hint += ": " + i.Fix.Action + " " + i.Fix.Value
			}
			_, _ = fmt.Fprintf(c.stdout, "issue:    %s (fix %s)\n", i.Code, hint)
		}
	}
}

// oidcPolicyFlags are the editable policy fields; only flags given on the
// command line overwrite the stored value.
type oidcPolicyFlags struct {
	claim, role, domains string
	jit, syncRoles       bool
}

func (f *oidcPolicyFlags) register(fs *flag.FlagSet) {
	fs.BoolVar(&f.jit, "jit", false, "create an account on first sign-in (default off: unknown users are refused)")
	fs.BoolVar(&f.syncRoles, "sync-roles", false, "reapply the mapped role on every sign-in (default off)")
	fs.StringVar(&f.role, "default-role", "", "role id for users no group mapping matches (default role_viewer, never a root role)")
	fs.StringVar(&f.claim, "groups-claim", "", "ID token claim holding group names (default groups, dotted paths allowed)")
	fs.StringVar(&f.domains, "allowed-domains", "", "comma-separated email domains allowed to sign in, also used to route the sign-in page")
}

func (f *oidcPolicyFlags) apply(fs *flag.FlagSet, req *apiclient.OAuthSSOPolicyRequest) {
	fs.Visit(func(fl *flag.Flag) {
		switch fl.Name {
		case "jit":
			req.JITProvisioning = f.jit
		case "sync-roles":
			req.SyncRoles = f.syncRoles
		case "default-role":
			req.DefaultRoleID = f.role
		case "groups-claim":
			req.GroupsClaim = f.claim
		case "allowed-domains":
			req.AllowedDomains = splitAndTrim(f.domains)
		}
	})
}

func (c cliCtx) runSSOOIDC(args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(c.stderr, ssoUsage(c.prog))
		return exitUsage
	}
	rest := args[1:]
	var f oidcPolicyFlags
	switch args[0] {
	case "list":
		return c.runAPI("sso oidc list", "sso oidc list [flags]", rest, nil,
			func(ctx context.Context, cl *Client, _ *flag.FlagSet) (any, func(), error) {
				ps, err := cl.ListOAuthSSOPolicies(ctx)
				return ps, c.printOIDCPolicies(ps...), err
			})
	case "get":
		return c.runAPI("sso oidc get", "sso oidc get <provider> [flags]", rest, nil,
			func(ctx context.Context, cl *Client, fs *flag.FlagSet) (any, func(), error) {
				provider, err := oneArg(fs, "provider")
				if err != nil {
					return nil, nil, err
				}
				p, err := cl.GetOAuthSSOPolicy(ctx, provider)
				return p, c.printOIDCPolicies(p), err
			})
	case "set":
		return c.runAPI("sso oidc set", "sso oidc set <provider> [--jit=BOOL] [--sync-roles=BOOL] [--default-role ID] [--groups-claim NAME] [--allowed-domains a.com,b.com] [flags]", rest, f.register,
			func(ctx context.Context, cl *Client, fs *flag.FlagSet) (any, func(), error) {
				provider, err := oneArg(fs, "provider")
				if err != nil {
					return nil, nil, err
				}
				cur, err := cl.GetOAuthSSOPolicy(ctx, provider)
				if err != nil {
					return nil, nil, err
				}
				req := apiclient.OAuthSSOPolicyRequest{GroupsClaim: cur.GroupsClaim, SyncRoles: cur.SyncRoles,
					DefaultRoleID: cur.DefaultRoleID, JITProvisioning: cur.JITProvisioning, AllowedDomains: cur.AllowedDomains}
				f.apply(fs, &req)
				p, err := cl.SetOAuthSSOPolicy(ctx, provider, req)
				return p, c.printOIDCPolicies(p), err
			})
	case "test":
		return c.runAPI("sso oidc test", "sso oidc test <provider> [flags]", rest, nil,
			func(ctx context.Context, cl *Client, fs *flag.FlagSet) (any, func(), error) {
				provider, err := oneArg(fs, "provider")
				if err != nil {
					return nil, nil, err
				}
				p, err := cl.GetOAuthSSOPolicy(ctx, provider)
				if err != nil {
					return nil, nil, err
				}
				if !p.Enabled {
					return nil, nil, newValidationError("enable provider %s before testing it", provider)
				}
				return printTestLogin(c, cl.URL("/api/v1/sso/oidc/"+apiclient.PathEscape(provider)+"/test/start"))
			})
	case "evaluate":
		var email, groups string
		var verified bool
		return c.runAPI("sso oidc evaluate", "sso oidc evaluate <provider> --email ADDRESS [--email-verified] [--groups g1,g2] [flags]", rest,
			func(fs *flag.FlagSet) {
				fs.StringVar(&email, "email", "", "email address the identity provider would assert")
				fs.BoolVar(&verified, "email-verified", false, "the provider marks the email verified")
				fs.StringVar(&groups, "groups", "", "comma-separated groups the ID token would carry")
			},
			func(ctx context.Context, cl *Client, fs *flag.FlagSet) (any, func(), error) {
				provider, err := oneArg(fs, "provider")
				if err != nil {
					return nil, nil, err
				}
				if strings.TrimSpace(email) == "" {
					return nil, nil, newValidationError("--email is required")
				}
				req := apiclient.OAuthSSOEvaluateRequest{Email: email, EmailVerified: verified}
				if flagWasSet(fs, "groups") {
					req.Groups = splitAndTrim(groups)
					if req.Groups == nil {
						req.Groups = []string{}
					}
				}
				e, err := cl.EvaluateOAuthSSO(ctx, provider, req)
				return e, c.printOIDCEvaluation(e), err
			})
	}
	_, _ = fmt.Fprintf(c.stderr, "%s: unknown sso oidc subcommand %q\n", c.prog, args[0])
	return exitUsage
}
