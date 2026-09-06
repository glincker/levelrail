// Package firewall manages host ufw (Uncomplicated Firewall) rules for
// ports this platform itself exposes: an app's HostPort pin or a managed
// database's public-access port. Before this package existed, the only
// code in this repo that ever wrote a firewall rule was install.sh's own
// opt-in LEVELRAIL_CONFIGURE_UFW step (see internal/api/doctor_firewall.go's
// former doc comment); everything else, including that doctor check, was
// read-only.
//
// Every rule this package creates is tagged with a "levelrail:" comment
// prefix (Rule.Comment, RuleComment). Sync only ever adds or removes rules
// carrying that prefix: an operator's own ufw rules, or a rule some other
// tool added, are never inspected for removal eligibility and never
// touched. This package also never calls "ufw enable" or changes ufw's
// default policy; if ufw isn't installed or isn't active, Sync and Report
// both report that plainly and do nothing, the same "don't guess, don't
// force a security posture change" stance install.sh's own opt-in flag
// already takes.
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

// Rule is one port this platform wants ufw to allow.
type Rule struct {
	// Port is the host port to allow inbound traffic on.
	Port int
	// Proto is "tcp" or "udp". Empty defaults to "tcp".
	Proto string
	// Owner identifies what this rule is for (e.g. "app:myapp",
	// "db:mydb"), stored as the ufw rule's comment with RuleCommentPrefix
	// prepended, and shown back in Report/Sync results so a caller can
	// tell which resource a given open (or missing) port belongs to.
	Owner string
}

func (r Rule) proto() string {
	if r.Proto == "" {
		return "tcp"
	}
	return r.Proto
}

func (r Rule) comment() string {
	return RuleCommentPrefix + r.Owner
}

func (r Rule) key() ruleKey {
	return ruleKey{port: r.Port, proto: r.proto()}
}

type ruleKey struct {
	port  int
	proto string
}

func (k ruleKey) portProto() string {
	return strconv.Itoa(k.port) + "/" + k.proto
}

// RuleStatus is one wanted Rule's actual state after Report or Sync.
type RuleStatus struct {
	Rule
	// Open is true if ufw currently allows this port/proto, whether this
	// package's own Sync opened it or an operator had already opened
	// this exact port/proto themselves outside this package (Open
	// reflects ufw's real state, not just "did we tag it").
	Open bool
}

// Result is Report's or Sync's outcome for one call.
type Result struct {
	// Installed is false when the ufw binary itself isn't found. Managed
	// is always empty and Applied/Removed are always zero in that case.
	Installed bool
	// Active is ufw's own "Status: active" flag. When false, ports
	// cannot be meaningfully reported as open or closed (ufw isn't
	// enforcing anything), so Managed still lists every wanted rule but
	// every Open is false and Applied/Removed are always zero: Sync
	// never calls "ufw enable" itself, see the package doc comment.
	Active bool
	// Managed is one RuleStatus per Rule passed to Report/Sync, in the
	// same order.
	Managed []RuleStatus
	// Extra is every ufw rule this package tagged (RuleCommentPrefix)
	// that Report/Sync found active but that wasn't in the caller's want
	// list, e.g. a port a deleted app/database left behind because Sync
	// hadn't run since. Sync removes these; Report only reports them.
	Extra []RuleStatus
	// Applied and Removed count the ufw commands Sync actually ran
	// (never populated by Report, which never mutates).
	Applied int
	Removed int
	// Errors carries one message per failed ufw invocation. Sync/Report
	// still return a nil error and whatever partial Result it has when
	// this is non-empty: one broken rule must not block every other
	// rule from being read or applied correctly, the same principle
	// every reconciler in this codebase already follows.
	Errors []string
}

// commandRunner abstracts exec.CommandContext so tests can supply
// fixture output instead of shelling out to a real ufw binary, the same
// fake-exec seam internal/api/doctor_firewall.go's own
// firewallCommandRunner and internal/telemetry/hostpatch.go's
// commandRunner already establish for other packages' system-command
// checks.
type commandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

func runRealCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput() //nolint:gosec // name/args are fixed literals or validated port numbers, never raw caller input
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

// newForTest builds a Manager over fake lookPath/run functions, used by
// this package's own tests and by internal/reconcile/firewall's tests
// via NewFake.
func newForTest(lookPath func(string) (string, error), run commandRunner) *Manager {
	return &Manager{lookPath: lookPath, run: run}
}

// Report computes the current state of every rule in want without
// mutating anything: a pure read, safe to call from an HTTP GET handler.
func (m *Manager) Report(ctx context.Context, want []Rule) (Result, error) {
	return m.diff(ctx, want)
}

// Sync converges ufw to match want exactly, for every rule this package
// itself tagged: it allows every wanted rule not already open, and
// removes every "levelrail:"-tagged rule that's open but no longer
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
		if status.Open {
			continue
		}
		if err := m.allow(ctx, status.Rule); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("allow %s (%s): %v", status.key().portProto(), status.Owner, err))
			continue
		}
		result.Managed[i].Open = true
		result.Applied++
	}

	var stillExtra []RuleStatus
	for _, status := range result.Extra {
		if err := m.delete(ctx, status.Rule); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("delete %s (%s): %v", status.key().portProto(), status.Owner, err))
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
		wantByKey[r.key()] = true
		if status, ok := existing[r.key()]; ok {
			managed[i].Open = true
			_ = status // existing[key] presence is enough; comment already validated as ours by parseUFWStatus
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
		extra = append(extra, RuleStatus{Rule: Rule{Port: k.port, Proto: k.proto, Owner: owner}, Open: true})
	}

	return Result{Installed: true, Active: true, Managed: managed, Extra: extra}, nil
}

func (m *Manager) allow(ctx context.Context, r Rule) error {
	_, err := m.run(ctx, "ufw", "allow", strconv.Itoa(r.Port)+"/"+r.proto(), "comment", r.comment())
	return err
}

func (m *Manager) delete(ctx context.Context, r Rule) error {
	_, err := m.run(ctx, "ufw", "delete", "allow", strconv.Itoa(r.Port)+"/"+r.proto())
	return err
}

// ufwRuleLine matches one data row of "ufw status verbose" output, e.g.
// "8080/tcp                   ALLOW IN    Anywhere" or the IPv6 twin
// "8080/tcp (v6)              ALLOW IN    Anywhere (v6)", capturing the
// port, an optional protocol, and (when present) a trailing "# comment".
var ufwRuleLine = regexp.MustCompile(`^(\d{1,5})(?:/(tcp|udp))?(?:\s*\(v6\))?\s+ALLOW\s+IN\s+.*?(?:#\s*(\S.*))?$`)

// parseUFWStatus extracts every "levelrail:"-tagged ALLOW rule from ufw
// status verbose's text output, keyed by port/proto. ufw prints a
// separate IPv4 and IPv6 line for a rule added without an explicit
// family; both lines carry the same comment, so the second write to the
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
		comment := strings.TrimSpace(m[3])
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
		out[ruleKey{port: port, proto: proto}] = comment
	}
	return out
}

func (s RuleStatus) key() ruleKey { return s.Rule.key() }
