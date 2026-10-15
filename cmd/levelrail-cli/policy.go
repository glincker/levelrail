package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func policyUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s policy list
  %[1]s policy set <org|project:ID|environment:ID> (--rule RULE=MODE[:VALUE] ... | --clear)
  %[1]s policy check [--app NAME] [--scope <org|project:ID|environment:ID> --rule ...] [--fail]

Deploy guardrails. MODE is enforce, warn or off. Rules:
  allowed_registries=enforce:ghcr.io,docker.io    require_readiness_probe=warn
  max_memory_mb=enforce:2048                      max_cpu=enforce:2
  forbid_privileged=enforce                       forbid_host_mounts=enforce[:/srv/data,/mnt]
  required_labels=warn:team,cost-center           max_replicas=enforce:5
set replaces the whole scope; the most specific scope wins per rule. check is a dry run
against current apps and never changes anything. --fail exits non-zero when any app would fail.
`, prog)
}

func runPolicy(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return dispatchSub(prog, "policy", policyUsage, policyCommands(), args, stdout, stderr, lookupEnv)
}

func policyCommands() map[string]apiCmd {
	return map[string]apiCmd{"list": policyListCommand(), "set": policySetCommand(), "check": policyCheckCommand()}
}

type ruleFlags []string

func (r *ruleFlags) String() string     { return strings.Join(*r, ",") }
func (r *ruleFlags) Set(v string) error { *r = append(*r, v); return nil }

// parsePolicyScope splits "org", "project:ID" or "environment:ID".
func parsePolicyScope(s string) (scope, id string, err error) {
	if s == "org" {
		return "org", "org", nil
	}
	scope, id, ok := strings.Cut(s, ":")
	if !ok || id == "" || (scope != "project" && scope != "environment") {
		return "", "", newValidationError("scope must be org, project:ID or environment:ID, got %q", s)
	}
	return scope, id, nil
}

// parsePolicyRules turns RULE=MODE[:VALUE] flags into a rule map.
func parsePolicyRules(flags []string) (map[string]apiclient.DeployPolicyRule, error) {
	out := map[string]apiclient.DeployPolicyRule{}
	for _, f := range flags {
		id, rest, ok := strings.Cut(f, "=")
		if !ok || id == "" {
			return nil, newValidationError("--rule must look like RULE=MODE[:VALUE], got %q", f)
		}
		mode, val, hasVal := strings.Cut(rest, ":")
		rule := apiclient.DeployPolicyRule{Mode: mode}
		if hasVal {
			switch id {
			case "max_memory_mb", "max_cpu", "max_replicas":
				n, err := strconv.ParseFloat(val, 64)
				if err != nil {
					return nil, newValidationError("rule %s needs a number, got %q", id, val)
				}
				rule.Limit = n
			default:
				rule.Values = strings.Split(val, ",")
			}
		}
		out[id] = rule
	}
	return out, nil
}

func policyListCommand() apiCmd {
	return apiCmd{
		label: "policy list",
		usage: policyUsage,
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			list, err := c.ListDeployPolicies(ctx)
			return list, func(w io.Writer) { printPolicies(w, list) }, err
		},
	}
}

func policySetCommand() apiCmd {
	var rules ruleFlags
	var clearAll bool
	return apiCmd{
		label: "policy set",
		usage: policyUsage,
		args:  1,
		setup: func(fs *flag.FlagSet) {
			fs.Var(&rules, "rule", "RULE=MODE[:VALUE], repeatable")
			fs.BoolVar(&clearAll, "clear", false, "remove every rule at this scope")
		},
		run: func(ctx context.Context, c *Client, pos []string) (any, func(io.Writer), error) {
			scope, id, err := parsePolicyScope(pos[0])
			if err != nil {
				return nil, nil, err
			}
			if clearAll {
				err := c.DeleteDeployPolicy(ctx, scope, id)
				return map[string]string{"cleared": pos[0]}, func(w io.Writer) { _, _ = fmt.Fprintf(w, "policy cleared for %s\n", pos[0]) }, err
			}
			parsed, err := parsePolicyRules(rules)
			if err != nil {
				return nil, nil, err
			}
			if len(parsed) == 0 {
				return nil, nil, newValidationError("give at least one --rule, or --clear")
			}
			res, err := c.SetDeployPolicy(ctx, scope, id, parsed)
			return res, func(w io.Writer) { printPolicies(w, []apiclient.DeployPolicy{res}) }, err
		},
	}
}

func policyCheckCommand() apiCmd {
	var (
		app, scopeArg string
		rules         ruleFlags
		failOnBlock   bool
	)
	return apiCmd{
		label: "policy check",
		usage: policyUsage,
		setup: func(fs *flag.FlagSet) {
			fs.StringVar(&app, "app", "", "check only this app")
			fs.StringVar(&scopeArg, "scope", "", "scope whose rules --rule previews instead of the stored ones")
			fs.Var(&rules, "rule", "RULE=MODE[:VALUE] to preview, repeatable")
			fs.BoolVar(&failOnBlock, "fail", false, "exit non-zero when any app would fail")
		},
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			req := apiclient.PolicyCheckRequest{App: app}
			if len(rules) > 0 || scopeArg != "" {
				scope, id, err := parsePolicyScope(scopeArg)
				if err != nil {
					return nil, nil, err
				}
				parsed, err := parsePolicyRules(rules)
				if err != nil {
					return nil, nil, err
				}
				req.Scope, req.ScopeID, req.Rules = scope, id, parsed
			}
			res, err := c.CheckDeployPolicy(ctx, req)
			if err == nil && failOnBlock && res.Failing > 0 {
				err = fmt.Errorf("%d of %d apps would fail deploy policy", res.Failing, res.Checked)
			}
			return res, func(w io.Writer) { printPolicyCheck(w, res) }, err
		},
	}
}

func printPolicies(out io.Writer, list []apiclient.DeployPolicy) {
	if len(list) == 0 {
		_, _ = fmt.Fprintln(out, "no deploy policies")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SCOPE\tRULE\tMODE\tVALUE")
	for _, p := range list {
		ids := make([]string, 0, len(p.Rules))
		for id := range p.Rules {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			r := p.Rules[id]
			val := strings.Join(r.Values, ",")
			if r.Limit != 0 {
				val = strconv.FormatFloat(r.Limit, 'f', -1, 64)
			}
			_, _ = fmt.Fprintf(tw, "%s:%s\t%s\t%s\t%s\n", p.Scope, p.ScopeID, id, r.Mode, val)
		}
	}
	_ = tw.Flush()
}

func printPolicyCheck(out io.Writer, res apiclient.PolicyCheckResult) {
	_, _ = fmt.Fprintf(out, "%d apps checked, %d would fail, %d with warnings\n", res.Checked, res.Failing, res.Warning)
	for _, a := range res.Apps {
		for _, v := range a.Violations {
			_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t%s\n  next: %s\n", a.App, v.Mode, v.Code, v.Message, v.NextAction)
		}
	}
}
