package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

func runDomainsShield(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return dispatchPolicy(prog, "shield", args, stdout, stderr, lookupEnv, func(args []string) policyVerb {
		switch args[0] {
		case "show":
			return runShieldShow
		case "set":
			return runShieldSet
		case "clear":
			return runShieldClear
		case "under-attack":
			return runShieldUnderAttack
		case "stand-down":
			return runShieldStandDown
		case "bots":
			return runShieldBots
		case "recommend":
			return runShieldRecommend
		}
		return nil
	}, domainsShieldUsage(prog))
}

func domainsShieldUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains shield show <app> <domain>
  %[1]s domains shield set <app> <domain> [--rate 120] [--burst 20] [--max-concurrent 10] [--max-body 1048576] [--block-bots] [flags]
  %[1]s domains shield clear <app> <domain>
  %[1]s domains shield under-attack <app> <domain> [--minutes 60]
  %[1]s domains shield stand-down <app> <domain>
  %[1]s domains shield bots
  %[1]s domains shield recommend <app> [--dry-run] [--block-bots]

Limits apply per client IP at the ingress. Rate limited and over-concurrency
requests get 429 with Retry-After, bad bots get 403, oversized bodies get 413.
Under attack tightens every limit and refuses listed bots until it expires.
Client addresses are held in a capped in-memory table and never logged.
`, prog)
}

func shieldResult(stdout, stderr io.Writer, inv policyInvocation, res apiclient.ShieldPolicyResource, err error) int {
	if err != nil {
		return reportError(stdout, stderr, inv.jsonOut, fmt.Errorf("shield for domain %q: %w", inv.domain, err))
	}
	return writeScheduledTaskResult(stdout, stderr, inv.of, res, func() { printShield(stdout, res) })
}

func printShield(out io.Writer, res apiclient.ShieldPolicyResource) {
	s := res.Spec
	if !res.Configured {
		_, _ = fmt.Fprintf(out, "%s: no abuse shield configured\n", res.Domain)
		return
	}
	state := "off"
	if s.Enabled {
		state = "on"
	}
	_, _ = fmt.Fprintf(out, "%s: shield %s, %d/min burst %d, %d concurrent, body limit %d bytes, bots %t, %d allowlisted\n",
		res.Domain, state, s.RatePerMinute, s.Burst, s.MaxConcurrent, s.MaxBodyBytes, s.BlockBots, len(s.Allow))
	if s.UnderAttack(time.Now()) {
		_, _ = fmt.Fprintf(out, "under attack until %s\n", s.UnderAttackUntil.Format(time.RFC3339))
	}
}

func runShieldShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{label: "domains shield show", argsHelp: "<app> <domain>", summary: "Shows a domain's abuse shield."})
	if !ok {
		return code
	}
	res, err := inv.client.GetDomainShield(bg(), inv.app, inv.domain)
	return shieldResult(stdout, stderr, inv, res, err)
}

func runShieldSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var s trafficpolicy.Shield
	var allow, agents string
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{
		label: "domains shield set", argsHelp: "<app> <domain>", summary: "Sets a domain's abuse shield. Omitted limits are off.",
		define: func(fs *flag.FlagSet) {
			fs.IntVar(&s.RatePerMinute, "rate", 0, "requests per minute per client IP (0 = off)")
			fs.IntVar(&s.Burst, "burst", 0, "burst size (default rate/6)")
			fs.IntVar(&s.MaxConcurrent, "max-concurrent", 0, "in-flight requests per client IP (0 = off)")
			fs.Int64Var(&s.MaxBodyBytes, "max-body", 0, "largest request body in bytes (0 = off)")
			fs.BoolVar(&s.BlockBots, "block-bots", false, "refuse user agents on the bot denylist")
			fs.StringVar(&agents, "bot-agents", "", "comma separated extra user agent substrings to refuse")
			fs.StringVar(&allow, "allow", "", "comma separated IPs or CIDRs the shield never touches")
			fs.IntVar(&s.RetryAfterSeconds, "retry-after", 0, "Retry-After seconds on 429 (0 = instance default)")
			fs.IntVar(&s.TarpitMillis, "tarpit-ms", 0, "delay rejections by this many milliseconds (0 = off)")
		},
	})
	if !ok {
		return code
	}
	s.Enabled = true
	s.Allow, s.BotAgents = splitCSV(allow), splitCSV(agents)
	if cur, err := inv.client.GetDomainShield(bg(), inv.app, inv.domain); err == nil {
		s.UnderAttackUntil = cur.Spec.UnderAttackUntil
	}
	res, err := inv.client.SetDomainShield(bg(), inv.app, inv.domain, s)
	return shieldResult(stdout, stderr, inv, res, err)
}

func runShieldClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{label: "domains shield clear", argsHelp: "<app> <domain>", summary: "Removes the abuse shield."})
	if !ok {
		return code
	}
	res, err := inv.client.ClearDomainShield(bg(), inv.app, inv.domain)
	return shieldResult(stdout, stderr, inv, res, err)
}

func runShieldUnderAttack(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var minutes int
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{
		label: "domains shield under-attack", argsHelp: "<app> <domain>", summary: "Tightens limits and refuses listed bots until it expires.",
		define: func(fs *flag.FlagSet) {
			fs.IntVar(&minutes, "minutes", 0, "how long to stay in emergency mode (0 = instance default)")
		},
	})
	if !ok {
		return code
	}
	res, err := inv.client.StartUnderAttack(bg(), inv.app, inv.domain, minutes)
	return shieldResult(stdout, stderr, inv, res, err)
}

func runShieldStandDown(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	inv, code, ok := parsePolicyCall(prog, args, stderr, lookupEnv, policyCmdSpec{label: "domains shield stand-down", argsHelp: "<app> <domain>", summary: "Ends emergency mode early."})
	if !ok {
		return code
	}
	res, err := inv.client.StopUnderAttack(bg(), inv.app, inv.domain)
	return shieldResult(stdout, stderr, inv, res, err)
}

func runShieldBots(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "domains shield bots", policyJSONUsage, stderr)
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return code
	}
	res, err := apiClientFromFlags(prog, apiURL, token, profile, lookupEnv).ShieldBots(bg())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("shield bot list: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintln(stdout, strings.Join(res.Agents, "\n"))
	})
}
