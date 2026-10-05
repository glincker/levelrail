package firewall

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func fakeLookPath(installed bool) func(string) (string, error) {
	return func(name string) (string, error) {
		if !installed {
			return "", exec.ErrNotFound
		}
		return "/usr/sbin/" + name, nil
	}
}

func TestManagerReport_NotInstalled(t *testing.T) {
	m := NewFake("acme:", fakeLookPath(false), func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("run should not be called when ufw is not installed")
		return nil, nil
	})
	result, err := m.Report(context.Background(), []Rule{{Port: 22, Owner: "manual:ssh"}})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if result.Installed {
		t.Fatal("expected Installed=false")
	}
}

func TestManagerReport_InactiveStillListsManaged(t *testing.T) {
	m := NewFake("acme:", fakeLookPath(true), func(context.Context, string, ...string) ([]byte, error) {
		return []byte("Status: inactive\n"), nil
	})
	want := []Rule{{Port: 8081, Owner: "rule:1"}}
	result, err := m.Report(context.Background(), want)
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if !result.Installed || result.Active {
		t.Fatalf("expected Installed=true, Active=false, got %+v", result)
	}
	if len(result.Managed) != 1 || result.Managed[0].Applied {
		t.Fatalf("expected one unmanaged rule, got %+v", result.Managed)
	}
}

const sampleUFWStatus = `Status: active

To                         Action      From
--                         ------      ----
8081/tcp                   ALLOW IN    Anywhere                   # acme:rule:1
22/tcp                     DENY IN     203.0.113.0/24              # acme:rule:2
9000/tcp                   ALLOW IN    Anywhere                   # some-other-tool
`

func TestManagerReport_ParsesTaggedRulesOnly(t *testing.T) {
	m := NewFake("acme:", fakeLookPath(true), func(context.Context, string, ...string) ([]byte, error) {
		return []byte(sampleUFWStatus), nil
	})
	want := []Rule{
		{Port: 8081, Proto: "tcp", Action: ActionAllow, Owner: "rule:1"},
		{Port: 22, Proto: "tcp", SourceCIDR: "203.0.113.0/24", Action: ActionDeny, Owner: "rule:2"},
	}
	result, err := m.Report(context.Background(), want)
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if !result.Active {
		t.Fatal("expected Active=true")
	}
	for i, status := range result.Managed {
		if !status.Applied {
			t.Errorf("rule %d (%s) expected Applied=true", i, status.Owner)
		}
	}
	if len(result.Extra) != 0 {
		t.Fatalf("expected no extra tagged rules, got %+v", result.Extra)
	}
}

func TestManagerSync_AppliesMissingAndRemovesExtra(t *testing.T) {
	var ran []string
	run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		ran = append(ran, "ufw "+strings.Join(args, " "))
		if len(args) > 0 && args[0] == "status" {
			return []byte(sampleUFWStatus), nil
		}
		return nil, nil
	}
	m := NewFake("acme:", fakeLookPath(true), run)

	want := []Rule{
		{Port: 8081, Proto: "tcp", Action: ActionAllow, Owner: "rule:1"},
		{Port: 5432, Proto: "tcp", SourceCIDR: "10.0.0.0/24", Action: ActionAllow, Owner: "rule:3"},
	}
	result, err := m.Sync(context.Background(), want)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Applied != 1 {
		t.Fatalf("expected 1 applied rule, got %d (%+v)", result.Applied, result)
	}
	if result.Removed != 1 {
		t.Fatalf("expected 1 removed rule (rule:2, no longer wanted), got %d (%+v)", result.Removed, result)
	}
	foundApply := false
	for _, cmd := range ran {
		if strings.Contains(cmd, "allow from 10.0.0.0/24 to any port 5432 proto tcp") {
			foundApply = true
		}
	}
	if !foundApply {
		t.Fatalf("expected an allow-from-CIDR invocation, got commands: %v", ran)
	}
}

// Half-succeeded case: one managed rule's apply fails, the other must
// still be attempted and succeed, matching every other reconciler's
// test for a partially-failed pass.
func TestManagerSync_HalfSucceeded(t *testing.T) {
	calls := 0
	run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "status" {
			return []byte("Status: active\n"), nil
		}
		calls++
		if calls == 1 {
			return nil, errors.New("ufw: could not acquire lock")
		}
		return nil, nil
	}
	m := NewFake("acme:", fakeLookPath(true), run)

	want := []Rule{
		{Port: 8081, Owner: "rule:1"},
		{Port: 8082, Owner: "rule:2"},
	}
	result, err := m.Sync(context.Background(), want)
	if err != nil {
		t.Fatalf("Sync should not return an error on a partial failure: %v", err)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected exactly one recorded error, got %v", result.Errors)
	}
	if result.Applied != 1 {
		t.Fatalf("expected the second rule to still apply despite the first failing, got Applied=%d", result.Applied)
	}
}

func TestValidate_RefusesDenyOnRequiredPort(t *testing.T) {
	r := Rule{Port: 9443, Action: ActionDeny, Owner: "rule:1"}
	err := Validate(r, []int{8080, 9443})
	if !errors.Is(err, ErrWouldLockOut) {
		t.Fatalf("expected ErrWouldLockOut, got %v", err)
	}
}

func TestValidate_RefusesCIDRScopedAllowOnRequiredPort(t *testing.T) {
	r := Rule{Port: 8080, SourceCIDR: "10.0.0.0/24", Action: ActionAllow, Owner: "rule:1"}
	err := Validate(r, []int{8080})
	if !errors.Is(err, ErrWouldLockOut) {
		t.Fatalf("expected ErrWouldLockOut, got %v", err)
	}
}

func TestValidate_AllowsUnrestrictedAllowOnRequiredPort(t *testing.T) {
	r := Rule{Port: 8080, Action: ActionAllow, Owner: "rule:1"}
	if err := Validate(r, []int{8080}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidate_AllowsAnyRuleOnNonRequiredPort(t *testing.T) {
	r := Rule{Port: 51820, Action: ActionDeny, SourceCIDR: "0.0.0.0/0", Owner: "rule:1"}
	if err := Validate(r, []int{8080, 9443, 80, 443}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestSyncRejectsEmptyPrefix(t *testing.T) {
	m := NewFake("", fakeLookPath(true), func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("ufw must not run with an empty prefix")
		return nil, nil
	})
	if _, err := m.Report(context.Background(), nil); err == nil {
		t.Fatal("expected error for empty prefix")
	}
}
