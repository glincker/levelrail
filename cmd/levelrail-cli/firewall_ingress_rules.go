package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/edgefw"
)

// parseWhen reads "FIELD[:NAME] OP [VALUE]" into a condition.
func parseWhen(s string) (edgefw.Condition, error) {
	fields := strings.Fields(s)
	if len(fields) < 2 {
		return edgefw.Condition{}, fmt.Errorf("condition %q needs FIELD OP VALUE", s)
	}
	field, name, _ := strings.Cut(fields[0], ":")
	c := edgefw.Condition{Field: edgefw.Field(field), Name: name, Op: edgefw.Op(fields[1])}
	if len(fields) > 2 {
		c.Value = strings.Join(fields[2:], " ")
	}
	if _, ok := edgefw.FieldOps[c.Field]; !ok {
		return edgefw.Condition{}, fmt.Errorf("unknown field %q in %q", field, s)
	}
	return c, nil
}

type whenList []string

func (w *whenList) String() string     { return strings.Join(*w, "; ") }
func (w *whenList) Set(v string) error { *w = append(*w, v); return nil }

type fwRuleFlags struct {
	name, note, scope, action string
	when                      whenList
	disabled                  bool
	rate, window              int
	force                     bool
}

func (rf *fwRuleFlags) define(fs *flag.FlagSet) {
	fs.StringVar(&rf.name, "name", "", "rule name")
	fs.StringVar(&rf.note, "note", "", "optional note")
	fs.StringVar(&rf.scope, "scope", "", "host the rule applies to (default: every host)")
	fs.StringVar(&rf.action, "action", "", "allow, deny, challenge, log or rate_limit")
	fs.Var(&rf.when, "when", `condition "FIELD OP VALUE", repeat to require all`)
	fs.BoolVar(&rf.disabled, "disabled", false, "create or keep the rule switched off")
	fs.IntVar(&rf.rate, "rate", 0, "rate_limit: requests allowed per window")
	fs.IntVar(&rf.window, "window", 0, "rate_limit: window in seconds")
	fs.BoolVar(&rf.force, "force", false, "confirm a rule that would block your own address")
}

func (rf *fwRuleFlags) apply(base edgefw.Rule, set func(string) bool) (edgefw.Rule, error) {
	r := base
	if set("name") {
		r.Name = rf.name
	}
	if set("note") {
		r.Note = rf.note
	}
	if set("scope") {
		r.Scope = rf.scope
	}
	if set("action") {
		r.Action = edgefw.Action(rf.action)
	}
	if set("rate") {
		r.RateCount = rf.rate
	}
	if set("window") {
		r.RateWindowSeconds = rf.window
	}
	if set("disabled") {
		r.Enabled = !rf.disabled
	}
	if len(rf.when) > 0 {
		r.Conditions = nil
		for _, w := range rf.when {
			c, err := parseWhen(w)
			if err != nil {
				return edgefw.Rule{}, err
			}
			r.Conditions = append(r.Conditions, c)
		}
	}
	return r, nil
}

func visited(fs *flag.FlagSet) func(string) bool {
	seen := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { seen[f.Name] = true })
	return func(n string) bool { return seen[n] }
}

func runFwRules(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, firewallIngressUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "list":
		c, code, ok := parseFirewallCall(prog, rest, stderr, lookupEnv, fwSpec{label: "firewall ingress rules list", summary: "Lists rules in evaluation order."})
		if !ok {
			return code
		}
		rules, err := c.client.ListFirewallEdgeRules(bg())
		if err != nil {
			return c.fail(stdout, stderr, "list firewall rules", err)
		}
		return c.emit(stdout, stderr, rules, func() { printFwRules(stdout, rules) })
	case "add":
		return runFwRuleAdd(prog, rest, stdout, stderr, lookupEnv)
	case "update":
		return runFwRuleUpdate(prog, rest, stdout, stderr, lookupEnv)
	case "delete":
		c, code, ok := parseFirewallCall(prog, rest, stderr, lookupEnv, fwSpec{label: "firewall ingress rules delete", argsHelp: "<id>", summary: "Deletes a rule.", nargs: 1})
		if !ok {
			return code
		}
		if err := c.client.DeleteFirewallEdgeRule(bg(), c.args[0]); err != nil {
			return c.fail(stdout, stderr, "delete firewall rule", err)
		}
		return c.emit(stdout, stderr, map[string]bool{"deleted": true}, func() { _, _ = fmt.Fprintf(stdout, "rule %s deleted\n", c.args[0]) })
	case "move":
		return runFwRuleMove(prog, rest, stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown rules subcommand %q\n\n%s", prog, args[0], firewallIngressUsage(prog))
	return exitUsage
}

func runFwRuleAdd(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var rf fwRuleFlags
	c, code, ok := parseFirewallCall(prog, args, stderr, lookupEnv, fwSpec{label: "firewall ingress rules add", summary: "Adds a rule at the end of the list.", define: rf.define})
	if !ok {
		return code
	}
	r, err := rf.apply(edgefw.Rule{Enabled: true}, func(string) bool { return true })
	if err != nil {
		return reportError(stdout, stderr, c.jsonOut, newValidationError("%s", err.Error()))
	}
	r.Enabled = !rf.disabled
	created, err := c.client.CreateFirewallEdgeRule(bg(), r, rf.force)
	if err != nil {
		return c.fail(stdout, stderr, "add firewall rule", err)
	}
	return c.emit(stdout, stderr, created, func() { _, _ = fmt.Fprintf(stdout, "rule %s added at position %d\n", created.ID, created.Position) })
}

func runFwRuleUpdate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var rf fwRuleFlags
	var fs *flag.FlagSet
	c, code, ok := parseFirewallCall(prog, args, stderr, lookupEnv, fwSpec{label: "firewall ingress rules update", argsHelp: "<id>", summary: "Changes the flags you pass and keeps the rest.", nargs: 1,
		define: func(f *flag.FlagSet) { fs = f; rf.define(f) }})
	if !ok {
		return code
	}
	rules, err := c.client.ListFirewallEdgeRules(bg())
	if err != nil {
		return c.fail(stdout, stderr, "load firewall rules", err)
	}
	var cur *apiclient.FirewallEdgeRule
	for i := range rules {
		if rules[i].ID == c.args[0] {
			cur = &rules[i]
		}
	}
	if cur == nil {
		return c.fail(stdout, stderr, "update firewall rule", errors.New("no rule with id "+c.args[0]))
	}
	r, err := rf.apply(cur.Rule, visited(fs))
	if err != nil {
		return reportError(stdout, stderr, c.jsonOut, newValidationError("%s", err.Error()))
	}
	updated, err := c.client.UpdateFirewallEdgeRule(bg(), cur.ID, r, rf.force)
	if err != nil {
		return c.fail(stdout, stderr, "update firewall rule", err)
	}
	return c.emit(stdout, stderr, updated, func() { _, _ = fmt.Fprintf(stdout, "rule %s updated\n", updated.ID) })
}

func runFwRuleMove(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var to int
	c, code, ok := parseFirewallCall(prog, args, stderr, lookupEnv, fwSpec{label: "firewall ingress rules move", argsHelp: "<id>", summary: "Moves a rule to a 1-based position.", nargs: 1,
		define: func(fs *flag.FlagSet) { fs.IntVar(&to, "to", 0, "new position, 1 is evaluated first (required)") }})
	if !ok {
		return code
	}
	rules, err := c.client.ListFirewallEdgeRules(bg())
	if err != nil {
		return c.fail(stdout, stderr, "load firewall rules", err)
	}
	ids, err := moveID(rules, c.args[0], to)
	if err != nil {
		return reportError(stdout, stderr, c.jsonOut, newValidationError("%s", err.Error()))
	}
	out, err := c.client.ReorderFirewallEdgeRules(bg(), ids)
	if err != nil {
		return c.fail(stdout, stderr, "move firewall rule", err)
	}
	return c.emit(stdout, stderr, out, func() { printFwRules(stdout, out) })
}

// moveID returns the rule ids in the order that places id at position to.
func moveID(rules []apiclient.FirewallEdgeRule, id string, to int) ([]string, error) {
	if to < 1 || to > len(rules) {
		return nil, fmt.Errorf("--to must be between 1 and %d", len(rules))
	}
	ids := make([]string, 0, len(rules))
	found := false
	for _, r := range rules {
		if r.ID == id {
			found = true
			continue
		}
		ids = append(ids, r.ID)
	}
	if !found {
		return nil, errors.New("no rule with id " + id)
	}
	return append(ids[:to-1], append([]string{id}, ids[to-1:]...)...), nil
}

func runFwIP(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var e apiclient.FirewallEdgeIPEntry
	var force bool
	c, code, ok := parseFirewallCall(prog, args, stderr, lookupEnv, fwSpec{label: "firewall ingress " + verb, argsHelp: "<ip-or-cidr>", nargs: 1,
		summary: ipVerbSummary(verb),
		define: func(fs *flag.FlagSet) {
			fs.StringVar(&e.Scope, "scope", "", "host the entry applies to (default: every host)")
			fs.StringVar(&e.Note, "note", "", "optional note")
			fs.BoolVar(&force, "force", false, "confirm an entry that would block your own address")
		}})
	if !ok {
		return code
	}
	e.IP = c.args[0]
	var rule apiclient.FirewallEdgeRule
	var err error
	if verb == "block" {
		rule, err = c.client.BlockFirewallEdgeIP(bg(), e, force)
	} else {
		rule, err = c.client.AllowFirewallEdgeIP(bg(), e)
	}
	if err != nil {
		return c.fail(stdout, stderr, verb+" "+e.IP, err)
	}
	return c.emit(stdout, stderr, rule, func() { _, _ = fmt.Fprintf(stdout, "%s: rule %s at position %d\n", verb, rule.ID, rule.Position) })
}

func fwOnOff(s string) (bool, error) {
	switch s {
	case "on":
		return true, nil
	case "off":
		return false, nil
	}
	return false, errors.New("use on or off")
}

func runFwBots(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || (args[0] != "show" && args[0] != "set") {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s firewall ingress bots show\n  %[1]s firewall ingress bots set [flags]\n", prog)
		return exitUsage
	}
	var bots, ai, agents, bans string
	var fs *flag.FlagSet
	c, code, ok := parseFirewallCall(prog, args[1:], stderr, lookupEnv, fwSpec{label: "firewall ingress bots " + args[0], summary: "Shows or sets bot handling and automatic bans.",
		define: func(f *flag.FlagSet) {
			fs = f
			if args[0] == "set" {
				f.StringVar(&bots, "bots", "", "bad bot list mode: off, log, challenge or block")
				f.StringVar(&ai, "ai-bots", "", "AI crawler list mode: off, log, challenge or block")
				f.StringVar(&agents, "ai-agents", "", "comma separated AI crawler user agent substrings (empty = shipped list)")
				f.StringVar(&bans, "bans", "", "automatic temporary bans: on or off")
			}
		}})
	if !ok {
		return code
	}
	cur, err := c.client.FirewallEdgeSettings(bg())
	if err != nil {
		return c.fail(stdout, stderr, "load firewall settings", err)
	}
	if args[0] == "set" {
		seen := visited(fs)
		s := cur.Settings
		if seen("bots") {
			s.BotMode = edgefw.Mode(bots)
		}
		if seen("ai-bots") {
			s.AIBotMode = edgefw.Mode(ai)
		}
		if seen("ai-agents") {
			s.AIBotAgents = splitCSV(agents)
		}
		if seen("bans") {
			if s.BansEnabled, err = fwOnOff(bans); err != nil {
				return reportError(stdout, stderr, c.jsonOut, newValidationError("--bans: %s", err.Error()))
			}
		}
		if cur, err = c.client.SetFirewallEdgeSettings(bg(), s); err != nil {
			return c.fail(stdout, stderr, "set firewall settings", err)
		}
	}
	return c.emit(stdout, stderr, cur, func() {
		_, _ = fmt.Fprintf(stdout, "bad bots: %s\nAI bots: %s (%d custom agents, %d in the shipped list)\nautomatic bans: %t\n",
			cur.BotMode, cur.AIBotMode, len(cur.AIBotAgents), len(cur.DefaultAIBotAgents), cur.BansEnabled)
	})
}

func runFwAttack(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	usage := func() int {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s firewall ingress attack-mode on [--minutes N] [--challenge]\n  %[1]s firewall ingress attack-mode off\n", prog)
		return exitUsage
	}
	if len(args) == 0 {
		return usage()
	}
	switch args[0] {
	case "on", "off":
	default:
		return usage()
	}
	var minutes int
	var challenge bool
	c, code, ok := parseFirewallCall(prog, args[1:], stderr, lookupEnv, fwSpec{label: "firewall ingress attack-mode " + args[0], summary: "Turns instance-wide attack mode on or off. It lapses by itself.",
		define: func(fs *flag.FlagSet) {
			if args[0] == "on" {
				fs.IntVar(&minutes, "minutes", 0, "how long to stay on (0 = instance default)")
				fs.BoolVar(&challenge, "challenge", false, "also challenge every visitor with the browser check")
			}
		}})
	if !ok {
		return code
	}
	var res apiclient.FirewallEdgeAttack
	var err error
	if args[0] == "on" {
		res, err = c.client.StartFirewallEdgeAttack(bg(), minutes, challenge)
	} else {
		res, err = c.client.StopFirewallEdgeAttack(bg())
	}
	if err != nil {
		return c.fail(stdout, stderr, "attack mode", err)
	}
	return c.emit(stdout, stderr, res, func() {
		if res.Active {
			_, _ = fmt.Fprintf(stdout, "attack mode on until %s (challenge %s)\n", res.Until, strconv.FormatBool(res.Challenge))
			return
		}
		_, _ = fmt.Fprintln(stdout, "attack mode off")
	})
}

func ipVerbSummary(verb string) string {
	if verb == "block" {
		return "Denies an address or range."
	}
	return "Lets an address or range skip every other rule."
}
