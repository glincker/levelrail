package pipeline

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

type countingSecrets struct{ resolved atomic.Int32 }

func (c *countingSecrets) Resolve(context.Context, string, string) (string, error) {
	c.resolved.Add(1)
	return "s3cr3t-value", nil
}

func prYAML(forks string) string {
	on := "on:\n  pull_request:\n    branches: [main]\n"
	if forks != "" {
		on += "    forks: " + forks + "\n"
	}
	return "version: 1\n" + on + "jobs:\n  t:\n    image: a\n    steps:\n      - run: echo $TOKEN\n        secrets: [TOKEN]\n"
}

func newTriggerHarness(t *testing.T) (*harness, *countingSecrets) {
	t.Helper()
	h := newHarness(t, func(string) execResult { return execResult{Output: "ok\n"} })
	sec := &countingSecrets{}
	h.e.Close()
	h.e = New(Config{
		Store: h.db, Actions: h.act, Secrets: sec, NamePrefix: "t",
		Runtime: func(string) (Runtime, error) { return h.rt, nil },
		NewID: func() string {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.seq++
			return fmt.Sprintf("id%03d", h.seq)
		},
	})
	t.Cleanup(func() { h.e.Close() })
	return h, sec
}

func (h *harness) trigger(ev Event) []store.PipelineRun {
	h.t.Helper()
	return h.e.TriggerEvent(context.Background(), "web", ev, "refs/heads/feat", "abc123", "webhook")
}

func (h *harness) triggerLog() []store.PipelineTriggerLog {
	h.t.Helper()
	log, err := h.db.ListPipelineTriggerLog(context.Background(), "web", 50)
	if err != nil {
		h.t.Fatal(err)
	}
	return log
}

func prEvent(fork bool) Event {
	return Event{Kind: TriggerPullRequest, Branch: "main", Fork: fork, HeadRepo: "mallory/app"}
}

func TestForkPullRequestBlockedByDefault(t *testing.T) {
	for _, forks := range []string{"", "block"} {
		t.Run("forks="+forks, func(t *testing.T) {
			h, sec := newTriggerHarness(t)
			h.save(prYAML(forks))
			if runs := h.trigger(prEvent(true)); len(runs) != 0 {
				t.Fatalf("fork PR started %d runs, want 0", len(runs))
			}
			list, _ := h.db.ListPipelineRuns(context.Background(), "web", "", 10)
			if len(list) != 0 {
				t.Fatalf("stored %d runs, want 0", len(list))
			}
			log := h.triggerLog()
			if len(log) != 1 || log[0].Decision != store.TriggerSkipped || !strings.Contains(log[0].Reason, "mallory/app") || !strings.Contains(log[0].Reason, "forks: block") {
				t.Fatalf("trigger log = %+v", log)
			}
			if sec.resolved.Load() != 0 {
				t.Fatal("secrets resolved for a blocked fork PR")
			}
		})
	}
}

func TestSameRepoPullRequestRuns(t *testing.T) {
	h, _ := newTriggerHarness(t)
	h.save(prYAML(""))
	runs := h.trigger(prEvent(false))
	if len(runs) != 1 {
		t.Fatalf("same-repo PR started %d runs, want 1", len(runs))
	}
	if r := h.done(runs[0].ID); r.Status != store.PipelineStatusSucceeded {
		t.Fatalf("run %s: %s", r.Status, r.Reason)
	}
	log := h.triggerLog()
	if len(log) != 1 || log[0].Decision != store.TriggerStarted || log[0].RunID != runs[0].ID {
		t.Fatalf("trigger log = %+v", log)
	}
}

func TestForkPullRequestAllowedWhenConfigured(t *testing.T) {
	h, _ := newTriggerHarness(t)
	h.save(prYAML("allow"))
	runs := h.trigger(prEvent(true))
	if len(runs) != 1 {
		t.Fatalf("allow started %d runs, want 1", len(runs))
	}
	if r := h.done(runs[0].ID); r.Status != store.PipelineStatusSucceeded {
		t.Fatalf("run %s: %s", r.Status, r.Reason)
	}
}

func TestForkPullRequestApproveHoldsSecretsUntilReleased(t *testing.T) {
	h, sec := newTriggerHarness(t)
	h.save(prYAML("approve"))
	runs := h.trigger(prEvent(true))
	if len(runs) != 1 || runs[0].HoldState != store.HoldPending {
		t.Fatalf("runs = %+v", runs)
	}
	id := runs[0].ID

	for range 5 {
		if err := h.e.Tick(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	held, _ := h.db.GetPipelineRun(context.Background(), id)
	if held.Status != store.PipelineStatusQueued || !strings.Contains(held.Reason, "waiting for approval") {
		t.Fatalf("held run = %s %q", held.Status, held.Reason)
	}
	if jobs, _ := h.db.ListPipelineJobs(context.Background(), id); len(jobs) != 0 {
		t.Fatalf("held run has %d jobs", len(jobs))
	}
	if h.rt.liveContainers() != 0 || len(h.rt.created) != 0 || sec.resolved.Load() != 0 {
		t.Fatalf("held run touched the runtime: containers=%d secrets=%d", len(h.rt.created), sec.resolved.Load())
	}
	if log := h.triggerLog(); len(log) != 1 || log[0].Decision != store.TriggerHeld {
		t.Fatalf("trigger log = %+v", log)
	}

	ok, err := h.e.DecideHold(context.Background(), id, true, "gagan")
	if err != nil || !ok {
		t.Fatalf("approve: ok=%v err=%v", ok, err)
	}
	if again, _ := h.e.DecideHold(context.Background(), id, false, "other"); again {
		t.Fatal("a decided hold accepted a second decision")
	}
	if r := h.done(id); r.Status != store.PipelineStatusSucceeded {
		t.Fatalf("released run %s: %s", r.Status, r.Reason)
	}
	if sec.resolved.Load() == 0 {
		t.Fatal("released run never resolved its secret")
	}
}

func TestForkPullRequestApproveRejectedNeverRuns(t *testing.T) {
	h, sec := newTriggerHarness(t)
	h.save(prYAML("approve"))
	id := h.trigger(prEvent(true))[0].ID
	if ok, err := h.e.DecideHold(context.Background(), id, false, "gagan"); err != nil || !ok {
		t.Fatalf("reject: ok=%v err=%v", ok, err)
	}
	r := h.done(id)
	if r.Status != store.PipelineStatusCancelled || !strings.Contains(r.Reason, "rejected by gagan") {
		t.Fatalf("run = %s %q", r.Status, r.Reason)
	}
	if sec.resolved.Load() != 0 || len(h.rt.created) != 0 {
		t.Fatal("rejected run touched the runtime")
	}
}

func TestPullRequestFilterMatchesTargetBranch(t *testing.T) {
	h, _ := newTriggerHarness(t)
	h.save(prYAML(""))
	// Source branch feat, target dev: the main filter must not match.
	if runs := h.trigger(Event{Kind: TriggerPullRequest, Branch: "dev"}); len(runs) != 0 {
		t.Fatalf("started %d runs for a non-matching target", len(runs))
	}
	log := h.triggerLog()
	if len(log) != 1 || log[0].Decision != store.TriggerSkipped || !strings.Contains(log[0].Reason, `"dev"`) {
		t.Fatalf("trigger log = %+v", log)
	}
}

func TestTriggerLogExplainsNoListeners(t *testing.T) {
	h, _ := newTriggerHarness(t)
	h.save(prYAML(""))
	if runs := h.trigger(Event{Kind: TriggerPush, Branch: "main"}); len(runs) != 0 {
		t.Fatalf("started %d runs", len(runs))
	}
	log := h.triggerLog()
	if len(log) != 1 || !strings.Contains(log[0].Reason, "listens for push") || log[0].Pipeline != "" {
		t.Fatalf("trigger log = %+v", log)
	}
}

func TestTriggerLogRecordsInvalidDefinitionAndBranchFilter(t *testing.T) {
	h, _ := newTriggerHarness(t)
	h.save("version: 1\non:\n  push:\n    branches: [main]\njobs:\n  t:\n    image: a\n    steps:\n      - run: echo\n")
	h.trigger(Event{Kind: TriggerPush, Branch: "dev"})
	log := h.triggerLog()
	if len(log) != 1 || !strings.Contains(log[0].Reason, "on.push.branches") {
		t.Fatalf("trigger log = %+v", log)
	}
}

func TestForkPolicyDefaultsAndValidation(t *testing.T) {
	var nilTrigger *PRTrigger
	if nilTrigger.ForkPolicy() != ForksBlock || (&PRTrigger{}).ForkPolicy() != ForksBlock {
		t.Fatal("fork policy must default to block")
	}
	if _, issues := Validate([]byte(prYAML("nonsense"))); len(issues) == 0 {
		t.Fatal("an unknown forks value must fail validation")
	}
	for _, v := range []string{"block", "approve", "allow"} {
		if _, issues := Validate([]byte(prYAML(v))); len(issues) != 0 {
			t.Fatalf("forks: %s rejected: %v", v, issues)
		}
	}
}
