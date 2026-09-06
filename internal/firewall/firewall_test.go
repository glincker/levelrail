package firewall

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func notFound(string) (string, error)   { return "", errors.New("not found") }
func found(name string) (string, error) { return "/usr/sbin/" + name, nil }

const activeDenyStatus = `Status: active
Logging: on (low)
Default: deny (incoming), allow (outgoing), disabled (routed)
New profiles: skip

To                         Action      From
--                         ------      ----
22/tcp                     ALLOW IN    Anywhere
8080/tcp                   ALLOW IN    Anywhere                   # levelrail:app:web
8080/tcp (v6)              ALLOW IN    Anywhere (v6)              # levelrail:app:web
9090/tcp                   ALLOW IN    Anywhere                   # levelrail:db:mydb
`

const inactiveStatus = `Status: inactive
`

// fakeRunner builds a commandRunner for Report-only tests: Report never
// mutates (see TestManager_Report_NeverMutates), so this helper only
// ever needs to answer "ufw status verbose".
func fakeRunner(t *testing.T, statusOutput string) commandRunner {
	t.Helper()
	return func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name != "ufw" {
			t.Fatalf("run() called with name %q, want ufw", name)
		}
		if len(args) >= 2 && args[0] == "status" {
			return []byte(statusOutput), nil
		}
		t.Fatalf("run() called with unexpected args %v", args)
		return nil, nil
	}
}

func TestManager_Report_UFWNotInstalled(t *testing.T) {
	m := newForTest(notFound, fakeRunner(t, ""))
	result, err := m.Report(context.Background(), []Rule{{Port: 8080, Owner: "app:web"}})
	if err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	if result.Installed {
		t.Error("Installed = true, want false")
	}
	if len(result.Managed) != 1 || result.Managed[0].Open {
		t.Errorf("Managed = %+v, want one unopened rule", result.Managed)
	}
}

func TestManager_Report_UFWInactive(t *testing.T) {
	m := newForTest(found, fakeRunner(t, inactiveStatus))
	result, err := m.Report(context.Background(), []Rule{{Port: 8080, Owner: "app:web"}})
	if err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	if !result.Installed {
		t.Error("Installed = false, want true")
	}
	if result.Active {
		t.Error("Active = true, want false")
	}
	if result.Managed[0].Open {
		t.Error("Managed[0].Open = true, want false when ufw is inactive")
	}
}

func TestManager_Report_ExistingTaggedRuleIsOpen(t *testing.T) {
	m := newForTest(found, fakeRunner(t, activeDenyStatus))
	result, err := m.Report(context.Background(), []Rule{{Port: 8080, Owner: "app:web"}})
	if err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	if !result.Active {
		t.Fatal("Active = false, want true")
	}
	if !result.Managed[0].Open {
		t.Error("Managed[0].Open = false, want true (an existing levelrail:app:web rule already allows 8080/tcp)")
	}
}

func TestManager_Report_UntaggedRuleOnSamePortIsNotConsideredOpen(t *testing.T) {
	// Port 22 is allowed in activeDenyStatus but carries no levelrail:
	// comment (an operator's own SSH rule): Report must never treat it
	// as satisfying a want list entry for the same port, since this
	// package never claims ownership of a rule it didn't tag itself.
	m := newForTest(found, fakeRunner(t, activeDenyStatus))
	result, err := m.Report(context.Background(), []Rule{{Port: 22, Owner: "app:ssh-like"}})
	if err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	if result.Managed[0].Open {
		t.Error("Managed[0].Open = true, want false for an untagged operator rule on the same port")
	}
}

func TestManager_Report_ExtraTaggedRuleNotInWantList(t *testing.T) {
	m := newForTest(found, fakeRunner(t, activeDenyStatus))
	result, err := m.Report(context.Background(), nil)
	if err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	if len(result.Extra) != 2 {
		t.Fatalf("Extra = %+v, want 2 (8080/tcp and 9090/tcp, both levelrail:-tagged)", result.Extra)
	}
	owners := map[string]bool{}
	for _, e := range result.Extra {
		owners[e.Owner] = true
	}
	if !owners["app:web"] || !owners["db:mydb"] {
		t.Errorf("Extra owners = %v, want app:web and db:mydb", owners)
	}
}

func TestManager_Report_NeverMutates(t *testing.T) {
	run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) > 0 && (args[0] == "allow" || args[0] == "delete") {
			t.Fatalf("Report() called a mutating ufw command: %v", args)
		}
		return []byte(activeDenyStatus), nil
	}
	if _, err := newForTest(found, run).Report(context.Background(), []Rule{{Port: 3000, Owner: "app:new"}}); err != nil {
		t.Fatalf("Report() error = %v", err)
	}
}

func TestManager_Sync_OpensMissingRule(t *testing.T) {
	var allowCalls []string
	run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "status" {
			return []byte(activeEmptyStatus), nil
		}
		if len(args) > 0 && args[0] == "allow" {
			allowCalls = append(allowCalls, strings.Join(args, " "))
			return []byte("Rule added"), nil
		}
		t.Fatalf("unexpected run() call: %v", args)
		return nil, nil
	}
	m := newForTest(found, run)

	result, err := m.Sync(context.Background(), []Rule{{Port: 3000, Proto: "tcp", Owner: "app:new"}})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if result.Applied != 1 {
		t.Errorf("Applied = %d, want 1", result.Applied)
	}
	if !result.Managed[0].Open {
		t.Error("Managed[0].Open = false after a successful Sync, want true")
	}
	if len(allowCalls) != 1 || !strings.Contains(allowCalls[0], "3000/tcp") || !strings.Contains(allowCalls[0], "levelrail:app:new") {
		t.Errorf("allowCalls = %v, want a call allowing 3000/tcp tagged levelrail:app:new", allowCalls)
	}
}

const activeStatusOneTaggedRule = `Status: active
Default: deny (incoming), allow (outgoing), disabled (routed)

To                         Action      From
--                         ------      ----
8080/tcp                   ALLOW IN    Anywhere                   # levelrail:app:web
`

func TestManager_Sync_LeavesAlreadyOpenRuleAlone(t *testing.T) {
	run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "status" {
			return []byte(activeStatusOneTaggedRule), nil
		}
		t.Fatalf("Sync() called a mutating command for an already-satisfied rule: %v", args)
		return nil, nil
	}
	m := newForTest(found, run)

	result, err := m.Sync(context.Background(), []Rule{{Port: 8080, Owner: "app:web"}})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if result.Applied != 0 {
		t.Errorf("Applied = %d, want 0 (8080/tcp is already open and tagged levelrail:app:web)", result.Applied)
	}
}

func TestManager_Sync_RemovesExtraTaggedRule(t *testing.T) {
	var deleteCalls []string
	run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "status" {
			return []byte(activeDenyStatus), nil
		}
		if len(args) > 0 && args[0] == "delete" {
			deleteCalls = append(deleteCalls, strings.Join(args, " "))
			return []byte("Rule deleted"), nil
		}
		t.Fatalf("unexpected run() call: %v", args)
		return nil, nil
	}
	m := newForTest(found, run)

	// want is empty: both 8080/tcp (app:web) and 9090/tcp (db:mydb) from
	// activeDenyStatus are now stale and should be removed. 22/tcp is
	// untagged and must never be touched.
	result, err := m.Sync(context.Background(), nil)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if result.Removed != 2 {
		t.Errorf("Removed = %d, want 2", result.Removed)
	}
	if len(result.Extra) != 0 {
		t.Errorf("Extra after Sync = %+v, want empty (both removed)", result.Extra)
	}
	for _, call := range deleteCalls {
		if strings.Contains(call, "22/tcp") {
			t.Errorf("deleteCalls contains a delete for 22/tcp, an untagged rule that must never be touched: %v", deleteCalls)
		}
	}
}

// activeEmptyStatus is an active ufw with no existing rules at all, used
// by TestManager_Sync_PartialFailure to prove one failed "ufw allow"
// doesn't stop a sibling rule from being applied.
const activeEmptyStatus = `Status: active
Default: deny (incoming), allow (outgoing), disabled (routed)

To                         Action      From
--                         ------      ----
`

func TestManager_Sync_PartialFailure(t *testing.T) {
	var allowed []string
	run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "status" {
			return []byte(activeEmptyStatus), nil
		}
		if len(args) > 0 && args[0] == "allow" {
			joined := strings.Join(args, " ")
			if strings.Contains(joined, "4000/tcp") {
				return nil, &exec.ExitError{}
			}
			allowed = append(allowed, joined)
			return []byte("Rule added"), nil
		}
		t.Fatalf("unexpected run() call: %v", args)
		return nil, nil
	}
	m := newForTest(found, run)

	result, err := m.Sync(context.Background(), []Rule{
		{Port: 4000, Owner: "app:broken"},
		{Port: 5000, Owner: "app:fine"},
	})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if result.Applied != 1 {
		t.Errorf("Applied = %d, want 1 (only the 5000/tcp rule succeeded)", result.Applied)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("Errors = %v, want exactly 1", result.Errors)
	}
	if result.Managed[0].Open {
		t.Error("Managed[0] (4000/tcp) Open = true, want false since its allow call failed")
	}
	if !result.Managed[1].Open {
		t.Error("Managed[1] (5000/tcp) Open = false, want true since its allow call succeeded")
	}
	if len(allowed) != 1 || !strings.Contains(allowed[0], "5000/tcp") {
		t.Errorf("allowed = %v, want exactly one call for 5000/tcp", allowed)
	}
}
