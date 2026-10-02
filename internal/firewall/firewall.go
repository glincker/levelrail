// Package firewall manages host ufw rules as declarative allow/deny
// records. Every rule it creates carries a "levelrail:" comment prefix
// (RuleCommentPrefix); Sync only ever adds or removes rules carrying
// that tag, never an operator's own ufw rules, never "ufw enable" or
// the default policy.
package firewall

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// RuleCommentPrefix marks a ufw rule as one this package manages. Any
// existing ufw rule without this prefix in its comment is never a
// candidate for removal, regardless of what Sync's want list contains.
const RuleCommentPrefix = "levelrail:"

// Action is the ufw action a Rule applies: allow lets traffic through,
// deny drops it silently.
type Action string

// The two Action values a Rule can hold.
const (
	ActionAllow Action = "allow"
	ActionDeny  Action = "deny"
)

// Rule is one host firewall rule this platform wants ufw to enforce.
type Rule struct {
	// Port is the host port the rule applies to.
	Port int
	// Proto is "tcp" or "udp". Empty defaults to "tcp".
	Proto string
	// SourceCIDR restricts the rule to traffic from this CIDR. Empty
	// means any source ("Anywhere" in ufw's own terminology).
	SourceCIDR string
	// Action is ActionAllow or ActionDeny.
	Action Action
	// Owner identifies what this rule is for (e.g. "rule:3",
	// "app:myapp"), stored as the ufw rule's comment with
	// RuleCommentPrefix prepended, and shown back in Report/Sync results
	// so a caller can tell which resource a given rule belongs to.
	Owner string
}

func (r Rule) proto() string {
	if r.Proto == "" {
		return "tcp"
	}
	return r.Proto
}

func (r Rule) action() Action {
	if r.Action == "" {
		return ActionAllow
	}
	return r.Action
}

func (r Rule) comment() string {
	return RuleCommentPrefix + r.Owner
}

func (r Rule) key() ruleKey {
	return ruleKey{port: r.Port, proto: r.proto(), source: normalizeSource(r.SourceCIDR), action: r.action()}
}

type ruleKey struct {
	port   int
	proto  string
	source string
	action Action
}

func (k ruleKey) portProto() string {
	return strconv.Itoa(k.port) + "/" + k.proto
}

func normalizeSource(cidr string) string {
	cidr = strings.TrimSpace(cidr)
	if cidr == "" || cidr == "0.0.0.0/0" {
		return ""
	}
	return cidr
}

// RuleStatus is one wanted Rule's actual state after Report or Sync.
type RuleStatus struct {
	Rule
	// Applied is true when ufw's current rule set already has an entry
	// matching this exact port/proto/source/action, whether this
	// package's own Sync applied it or an operator had already set up
	// the identical rule outside this package.
	Applied bool
}

// Result is Report's or Sync's outcome for one call.
type Result struct {
	// Installed is false when the ufw binary itself isn't found. Managed
	// is always empty and Applied/Removed are always zero in that case.
	Installed bool
	// Active is ufw's own "Status: active" flag. When false, rules
	// cannot be meaningfully reported as applied (ufw isn't enforcing
	// anything), so Managed still lists every wanted rule but every
	// Applied is false and the Applied/Removed counters stay zero: Sync
	// never calls "ufw enable" itself, see the package doc comment.
	Active bool
	// Managed is one RuleStatus per Rule passed to Report/Sync, in the
	// same order.
	Managed []RuleStatus
	// Extra is every ufw rule this package tagged (RuleCommentPrefix)
	// that Report/Sync found active but that wasn't in the caller's want
	// list, e.g. a rule a deleted record left behind because Sync
	// hadn't run since. Sync removes these; Report only reports them.
	Extra []RuleStatus
	// Applied and Removed count the ufw commands Sync actually ran
	// (never populated by Report, which never mutates).
	Applied int
	Removed int
	// Errors carries one message per failed ufw invocation. Sync/Report
	// still return a nil error and whatever partial Result it has when
	// this is non-empty: one broken rule must not block every other
	// rule from being read or applied correctly.
	Errors []string
}

// commandRunner abstracts exec.CommandContext so tests can supply
// fixture output instead of shelling out to a real ufw binary, the same
// fake-exec seam internal/api/doctor_firewall.go's own
// firewallCommandRunner already establishes for a different package's
// system-command check. This is ordinary firewall-tool invocation via
// os/exec, not the docker-CLI-shelling anti-pattern this codebase bans
// elsewhere: ufw has no Go client library, the same reason
// doctor_firewall.go already shells out to it read-only.
type commandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

func runRealCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput() //nolint:gosec // name is the fixed literal "ufw"; args are validated port numbers, CIDRs, and owner labels, never raw shell input
}

// Manager syncs a desired set of Rules against the local host's ufw.
type Manager struct {
	lookPath func(string) (string, error)
	run      commandRunner
}

// New builds a Manager that shells out to the real "ufw" binary.
func New() *Manager {
	return &Manager{lookPath: exec.LookPath, run: runRealCommand}
}

// NewFake builds a Manager over fake lookPath/run functions, exported so
// internal/reconcile/firewall's own tests can exercise Sync without a
// real ufw binary, the same seam internal/firewall's own tests use.
func NewFake(lookPath func(string) (string, error), run commandRunner) *Manager {
	return &Manager{lookPath: lookPath, run: run}
}

// Report computes the current state of every rule in want without
// mutating anything: a pure read, safe to call from an HTTP GET handler.
func (m *Manager) Report(ctx context.Context, want []Rule) (Result, error) {
	return m.diff(ctx, want)
}

// Sync converges ufw to match want exactly, for every rule this package
// itself tagged: it applies every wanted rule not already present, and
// removes every "levelrail:"-tagged rule that's present but no longer
// wanted. It never touches a rule without that tag, never calls "ufw
// enable", and never changes ufw's default policy. Returns a nil error
// even when individual ufw invocations failed; see Result.Errors.
func (m *Manager) Sync(ctx context.Context, want []Rule) (Result, error) {
	result, err := m.diff(ctx, want)
	if err != nil {
		return result, err
	}
	if !result.Installed || !result.Active {
		return result, nil
	}

	for i, status := range result.Managed {
		if status.Applied {
			continue
		}
		if err := m.apply(ctx, status.Rule); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s %s (%s): %v", status.action(), status.key().portProto(), status.Owner, err))
			continue
		}
		result.Managed[i].Applied = true
		result.Applied++
	}

	var stillExtra []RuleStatus
	for _, status := range result.Extra {
		if err := m.delete(ctx, status.Rule); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("delete %s %s (%s): %v", status.action(), status.key().portProto(), status.Owner, err))
			stillExtra = append(stillExtra, status)
			continue
		}
		result.Removed++
	}
	result.Extra = stillExtra

	return result, nil
}

func (m *Manager) diff(ctx context.Context, want []Rule) (Result, error) {
	managed := make([]RuleStatus, len(want))
	for i, r := range want {
		managed[i] = RuleStatus{Rule: r}
	}

	if _, err := m.lookPath("ufw"); err != nil {
		return Result{Installed: false, Managed: managed}, nil
	}

	out, err := m.run(ctx, "ufw", "status", "verbose")
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return Result{Installed: true, Managed: managed}, fmt.Errorf("firewall: ufw status exited non-zero, may need elevated privileges: %w", err)
		}
		return Result{Installed: true, Managed: managed}, fmt.Errorf("firewall: ufw status: %w", err)
	}

	text := string(out)
	active := strings.Contains(text, "Status: active")
	existing := parseUFWStatus(text)

	if !active {
		return Result{Installed: true, Active: false, Managed: managed}, nil
	}

	wantByKey := make(map[ruleKey]bool, len(want))
	for i, r := range want {
		key := r.key()
		wantByKey[key] = true
		if _, ok := existing[key]; ok {
			managed[i].Applied = true
		}
	}

	var extra []RuleStatus
	extraKeys := make([]ruleKey, 0, len(existing))
	for k := range existing {
		if !wantByKey[k] {
			extraKeys = append(extraKeys, k)
		}
	}
	sort.Slice(extraKeys, func(i, j int) bool { return extraKeys[i].portProto() < extraKeys[j].portProto() })
	for _, k := range extraKeys {
		owner := strings.TrimPrefix(existing[k], RuleCommentPrefix)
		extra = append(extra, RuleStatus{Rule: Rule{Port: k.port, Proto: k.proto, SourceCIDR: k.source, Action: k.action, Owner: owner}, Applied: true})
	}

	return Result{Installed: true, Active: true, Managed: managed, Extra: extra}, nil
}

func (m *Manager) apply(ctx context.Context, r Rule) error {
	args := []string{string(r.action())}
	if src := normalizeSource(r.SourceCIDR); src != "" {
		args = append(args, "from", src, "to", "any", "port", strconv.Itoa(r.Port), "proto", r.proto())
	} else {
		args = append(args, strconv.Itoa(r.Port)+"/"+r.proto())
	}
	args = append(args, "comment", r.comment())
	_, err := m.run(ctx, "ufw", args...)
	return err
}

func (m *Manager) delete(ctx context.Context, r Rule) error {
	args := []string{"delete", string(r.action())}
	if src := normalizeSource(r.SourceCIDR); src != "" {
		args = append(args, "from", src, "to", "any", "port", strconv.Itoa(r.Port), "proto", r.proto())
	} else {
		args = append(args, strconv.Itoa(r.Port)+"/"+r.proto())
	}
	_, err := m.run(ctx, "ufw", args...)
	return err
}

func (s RuleStatus) action() Action { return s.Rule.action() }

// ufwRuleLine matches one data row of "ufw status verbose" output, e.g.
// "8080/tcp                   ALLOW IN    Anywhere" or
// "22/tcp                     DENY IN     203.0.113.0/24               # levelrail:rule:3",
// capturing the port, an optional protocol, the action, the source, and
// (when present) a trailing "# comment".
var ufwRuleLine = regexp.MustCompile(`^(\d{1,5})(?:/(tcp|udp))?\s+(ALLOW|DENY)\s+IN\s+(.+?)(?:\s+#\s*(\S.*))?$`)

// parseUFWStatus extracts every "levelrail:"-tagged rule from ufw status
// verbose's text output, keyed by port/proto/source/action. ufw prints a
// separate IPv4 and IPv6 line for a rule added without an explicit source
// CIDR; both lines carry the same comment, so the second write to the
// same key is harmless. Lines with no comment, or a comment not carrying
// RuleCommentPrefix, are real ufw rules this package must never consider
// for removal, so they're simply not added to the returned map.
func parseUFWStatus(text string) map[ruleKey]string {
	out := map[ruleKey]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		m := ufwRuleLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		comment := strings.TrimSpace(m[5])
		if !strings.HasPrefix(comment, RuleCommentPrefix) {
			continue
		}
		port, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		proto := m[2]
		if proto == "" {
			proto = "tcp"
		}
		action := ActionAllow
		if m[3] == "DENY" {
			action = ActionDeny
		}
		source := strings.TrimSuffix(strings.TrimSpace(m[4]), "(v6)")
		source = strings.TrimSpace(source)
		if source == "Anywhere" {
			source = ""
		}
		out[ruleKey{port: port, proto: proto, source: source, action: action}] = comment
	}
	return out
}
