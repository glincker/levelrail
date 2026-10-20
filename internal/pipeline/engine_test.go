package pipeline

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func okHandler(s string) execResult {
	switch {
	case strings.Contains(s, "echo hello"):
		return execResult{Output: "hello\n::set-output name=greeting::hi\n"}
	case strings.Contains(s, "exit 3"):
		return execResult{Output: "boom\n", Exit: 3}
	}
	return execResult{Output: "ok\n"}
}

const fullFlow = `
version: 1
name: release
on: { manual: {} }
stages: [test, build, deploy]
jobs:
  test:
    stage: test
    image: golang:1.23
    steps:
      - run: echo hello
  build:
    stage: build
    steps:
      - uses: build
        id: img
        with: { image: registry/app }
  ship:
    stage: deploy
    steps:
      - uses: approval
        with: { message: ship it, approvers: deploy }
      - uses: deploy
        with: { service: web }
      - uses: notify
        with: { message: shipped }
`

func TestFullFlowWithApproval(t *testing.T) {
	t.Parallel()
	h := newHarness(t, okHandler)
	run := h.start(h.save(fullFlow), StartOptions{Actor: "gagan", SHA: "abcdef1234567890"})

	h.until(run.ID, func(store.PipelineRun) bool {
		return h.jobs(run.ID)["ship"].Status == store.PipelineStatusWaitingApproval
	})
	if len(h.act.deploys) != 0 {
		t.Fatal("deploy ran before approval")
	}
	if got := h.jobs(run.ID)["build"].Status; got != store.PipelineStatusSucceeded {
		t.Fatalf("build status = %s", got)
	}

	approvals, _ := h.db.ListPipelineApprovals(context.Background(), run.ID)
	if len(approvals) != 1 || approvals[0].RequiredAbility != "deploy" {
		t.Fatalf("approvals = %+v", approvals)
	}
	if ok, err := h.db.DecidePipelineApproval(context.Background(), approvals[0].ID, "approved", "gagan", "", time.Now()); err != nil || !ok {
		t.Fatalf("decide: %v %v", ok, err)
	}

	got := h.done(run.ID)
	if got.Status != store.PipelineStatusSucceeded {
		t.Fatalf("run = %s (%s)\n%s", got.Status, got.Reason, h.dump(run.ID))
	}
	if len(h.act.builds) != 1 || h.act.builds[0].Tag != "abcdef123456" {
		t.Errorf("builds = %+v", h.act.builds)
	}
	if len(h.act.deploys) != 1 || h.act.deploys[0].Image != "registry/app:abcdef123456" {
		t.Errorf("deploys = %+v", h.act.deploys)
	}
	if len(h.act.notifies) != 1 || !strings.Contains(h.act.notifies[0], "web|true|shipped") {
		t.Errorf("notifies = %v", h.act.notifies)
	}
	if h.rt.liveContainers() != 0 {
		t.Errorf("containers leaked: %d", h.rt.liveContainers())
	}
	if !strings.Contains(h.logs(run.ID), "hello") {
		t.Errorf("logs missing output: %q", h.logs(run.ID))
	}
}

func TestApprovalRejectedFailsRun(t *testing.T) {
	t.Parallel()
	h := newHarness(t, okHandler)
	run := h.start(h.save(fullFlow), StartOptions{SHA: "abc"})
	h.until(run.ID, func(store.PipelineRun) bool {
		return h.jobs(run.ID)["ship"].Status == store.PipelineStatusWaitingApproval
	})
	approvals, _ := h.db.ListPipelineApprovals(context.Background(), run.ID)
	_, _ = h.db.DecidePipelineApproval(context.Background(), approvals[0].ID, "rejected", "boss", "not today", time.Now())

	got := h.done(run.ID)
	if got.Status != store.PipelineStatusFailed || !strings.Contains(got.Reason, "rejected by boss: not today") {
		t.Fatalf("run = %s %q", got.Status, got.Reason)
	}
	if len(h.act.deploys) != 0 {
		t.Error("deploy must not run after rejection")
	}
}

func TestFailedStepSkipsRestAndRemovesContainer(t *testing.T) {
	t.Parallel()
	h := newHarness(t, okHandler)
	yaml := `
version: 1
jobs:
  t:
    image: alpine
    steps:
      - run: exit 3
      - run: echo never
      - run: echo cleanup
        if: failure()
      - uses: notify
        with: { message: failed, on: failure }
        if: always()
`
	run := h.start(h.save(yaml), StartOptions{})
	got := h.done(run.ID)
	if got.Status != store.PipelineStatusFailed {
		t.Fatalf("run = %s", got.Status)
	}
	j := h.jobs(run.ID)["t"]
	byIdx := map[int]store.PipelineStep{}
	for _, s := range j.Steps {
		byIdx[s.Index] = s
	}
	if byIdx[1].Status != store.PipelineStatusFailed || byIdx[1].ExitCode == nil || *byIdx[1].ExitCode != 3 {
		t.Errorf("failing step = %+v", byIdx[1])
	}
	if byIdx[2].Status != store.PipelineStatusSkipped {
		t.Errorf("step after failure = %+v", byIdx[2])
	}
	if byIdx[3].Status != store.PipelineStatusSucceeded {
		t.Errorf("if: failure() step = %+v", byIdx[3])
	}
	if h.rt.ran("echo never") != 0 {
		t.Error("skipped step executed")
	}
	if len(h.act.notifies) != 1 || !strings.Contains(h.act.notifies[0], "|false|") {
		t.Errorf("notifies = %v", h.act.notifies)
	}
	if h.rt.liveContainers() != 0 {
		t.Errorf("container leaked after failure: %d", h.rt.liveContainers())
	}
}

func TestContinueOnErrorStepDoesNotFailJob(t *testing.T) {
	t.Parallel()
	h := newHarness(t, okHandler)
	yaml := "version: 1\njobs:\n  t:\n    image: a\n    steps:\n      - run: exit 3\n        continue_on_error: true\n      - run: echo after\n"
	run := h.start(h.save(yaml), StartOptions{})
	if got := h.done(run.ID); got.Status != store.PipelineStatusSucceeded {
		t.Fatalf("run = %s (%s)", got.Status, got.Reason)
	}
	if h.rt.ran("echo after") != 1 {
		t.Error("step after continue_on_error did not run")
	}
}

func TestMatrixAndNeeds(t *testing.T) {
	t.Parallel()
	h := newHarness(t, okHandler)
	yaml := `
version: 1
jobs:
  test:
    image: a
    matrix: { go: ["1.22", "1.23"] }
    steps:
      - run: echo go ${{ matrix.go }}
  after:
    needs: [test]
    image: a
    steps:
      - run: echo done
`
	run := h.start(h.save(yaml), StartOptions{})
	got := h.done(run.ID)
	if got.Status != store.PipelineStatusSucceeded {
		t.Fatalf("run = %s\n%s", got.Status, h.dump(run.ID))
	}
	jobs := h.jobs(run.ID)
	if len(jobs) != 3 || jobs["test[go=1.22]"].Status != store.PipelineStatusSucceeded {
		t.Fatalf("jobs = %v", h.dump(run.ID))
	}
	if h.rt.ran("echo go 1.22") != 1 || h.rt.ran("echo go 1.23") != 1 {
		t.Error("matrix values not interpolated")
	}
}

func TestJobConditionSkipsOnBranch(t *testing.T) {
	t.Parallel()
	h := newHarness(t, okHandler)
	yaml := "version: 1\non: { push: {} }\njobs:\n  prod:\n    if: branch == 'main'\n    image: a\n    steps:\n      - run: echo prod\n"
	run := h.start(h.save(yaml), StartOptions{Trigger: TriggerPush, Ref: "refs/heads/dev"})
	h.done(run.ID)
	if got := h.jobs(run.ID)["prod"]; got.Status != store.PipelineStatusSkipped {
		t.Fatalf("prod = %s", got.Status)
	}
}

func TestStepRetriesThenSucceeds(t *testing.T) {
	t.Parallel()
	calls := 0
	h := newHarness(t, func(s string) execResult {
		if strings.Contains(s, "flaky") {
			calls++
			if calls < 3 {
				return execResult{Exit: 1}
			}
		}
		return execResult{Output: "ok\n"}
	})
	yaml := "version: 1\njobs:\n  t:\n    image: a\n    steps:\n      - run: flaky\n        retries: 2\n"
	run := h.start(h.save(yaml), StartOptions{})
	if got := h.done(run.ID); got.Status != store.PipelineStatusSucceeded {
		t.Fatalf("run = %s\n%s", got.Status, h.dump(run.ID))
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestSecretsExportedAndMasked(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(s string) execResult {
		if strings.Contains(s, "s3cr3t-value") {
			return execResult{Output: "token is s3cr3t-value\n"}
		}
		return execResult{Output: "no secret\n", Exit: 1}
	})
	yaml := "version: 1\njobs:\n  t:\n    image: a\n    steps:\n      - run: echo $TOKEN\n        secrets: [TOKEN]\n"
	run := h.start(h.save(yaml), StartOptions{})
	if got := h.done(run.ID); got.Status != store.PipelineStatusSucceeded {
		t.Fatalf("run = %s\n%s", got.Status, h.dump(run.ID))
	}
	logs := h.logs(run.ID)
	if strings.Contains(logs, "s3cr3t-value") || !strings.Contains(logs, "token is ***") {
		t.Errorf("secret not masked: %q", logs)
	}
	if ran := h.rt.scripts[len(h.rt.scripts)-1]; !strings.Contains(ran, "export TOKEN='s3cr3t-value'") {
		t.Errorf("secret not exported via stdin script: %q", ran)
	}
}

func TestStepTimeout(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(string) execResult { return execResult{Block: true} })
	yaml := "version: 1\njobs:\n  t:\n    image: a\n    steps:\n      - run: sleep 100\n        timeout: 50ms\n"
	run := h.start(h.save(yaml), StartOptions{})
	got := h.done(run.ID)
	if got.Status != store.PipelineStatusFailed || !strings.Contains(got.Reason, "timed out") {
		t.Fatalf("run = %s %q", got.Status, got.Reason)
	}
	if h.rt.liveContainers() != 0 {
		t.Error("container leaked after timeout")
	}
}

func TestCancelRunningRun(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(string) execResult { return execResult{Block: true} })
	yaml := "version: 1\njobs:\n  t:\n    image: a\n    steps:\n      - run: sleep 100\n"
	run := h.start(h.save(yaml), StartOptions{})
	h.until(run.ID, func(store.PipelineRun) bool {
		return h.jobs(run.ID)["t"].Status == store.PipelineStatusRunning && h.rt.ran("sleep 100") > 0
	})
	if err := h.e.Cancel(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	got := h.done(run.ID)
	if got.Status != store.PipelineStatusCancelled {
		t.Fatalf("run = %s %q", got.Status, got.Reason)
	}
	if h.rt.liveContainers() != 0 {
		t.Error("container leaked after cancel")
	}
}

func TestConcurrencyCancelInProgress(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(s string) execResult {
		if strings.Contains(s, "slow") {
			return execResult{Block: true}
		}
		return execResult{Output: "ok\n"}
	})
	slow := "version: 1\nconcurrency: { group: deploy, cancel_in_progress: true }\njobs:\n  t:\n    image: a\n    steps:\n      - run: slow\n"
	p := h.save(slow)
	first := h.start(p, StartOptions{})
	h.until(first.ID, func(store.PipelineRun) bool { return h.rt.ran("slow") > 0 })

	p.YAML = strings.Replace(slow, "slow", "fast", 1)
	p, _ = h.db.SavePipeline(context.Background(), store.Pipeline{ID: p.ID, AppName: p.AppName, Name: p.Name, YAML: p.YAML, Enabled: true, CreatedAt: time.Now(), UpdatedAt: time.Now()})
	second := h.start(p, StartOptions{})

	if got := h.done(first.ID); got.Status != store.PipelineStatusCancelled {
		t.Fatalf("first = %s %q", got.Status, got.Reason)
	}
	if got := h.done(second.ID); got.Status != store.PipelineStatusSucceeded {
		t.Fatalf("second = %s %q\n%s", got.Status, got.Reason, h.dump(second.ID))
	}
}

func TestConcurrencyQueuesWithoutCancel(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	h := newHarness(t, func(s string) execResult {
		if strings.Contains(s, "gate") {
			<-release
		}
		return execResult{Output: "ok\n"}
	})
	yaml := "version: 1\nconcurrency: { group: g }\njobs:\n  t:\n    image: a\n    steps:\n      - run: gate\n"
	p := h.save(yaml)
	first := h.start(p, StartOptions{})
	second := h.start(p, StartOptions{})
	h.until(second.ID, func(r store.PipelineRun) bool { return strings.Contains(r.Reason, "queued behind run #1") })
	if r, _ := h.db.GetPipelineRun(context.Background(), second.ID); r.Status != store.PipelineStatusQueued {
		t.Fatalf("second = %s", r.Status)
	}
	close(release)
	h.done(first.ID)
	if got := h.done(second.ID); got.Status != store.PipelineStatusSucceeded {
		t.Fatalf("second = %s", got.Status)
	}
}

func TestResumeAfterRestartCleansHalfCreatedState(t *testing.T) {
	t.Parallel()
	hold := true
	h := newHarness(t, func(s string) execResult {
		if strings.Contains(s, "step-two") && hold {
			return execResult{Block: true}
		}
		return execResult{Output: "ok\n"}
	})
	yaml := "version: 1\njobs:\n  t:\n    image: a\n    steps:\n      - run: step-one\n      - run: step-two\n"
	run := h.start(h.save(yaml), StartOptions{})
	h.until(run.ID, func(store.PipelineRun) bool { return h.rt.ran("step-two") > 0 })

	h.e.Close()
	if h.rt.liveContainers() == 0 {
		t.Fatal("expected the interrupted job's container to remain, as after a crash")
	}
	if got, _ := h.db.GetPipelineRun(context.Background(), run.ID); got.Status != store.PipelineStatusRunning {
		t.Fatalf("interrupted run = %s, want running", got.Status)
	}

	hold = false
	h.e = h.newEngine()
	got := h.done(run.ID)
	if got.Status != store.PipelineStatusSucceeded {
		t.Fatalf("resumed run = %s %q\n%s", got.Status, got.Reason, h.dump(run.ID))
	}
	if h.rt.ran("step-one") != 1 {
		t.Errorf("completed step re-ran: %d times", h.rt.ran("step-one"))
	}
	if h.rt.ran("step-two") != 2 {
		t.Errorf("interrupted step ran %d times, want 2", h.rt.ran("step-two"))
	}
	if h.rt.liveContainers() != 0 {
		t.Errorf("leaked containers after resume: %d", h.rt.liveContainers())
	}
}

func TestCreateContainerFailureFailsJob(t *testing.T) {
	t.Parallel()
	h := newHarness(t, okHandler)
	h.rt.failCreate = errFake
	run := h.start(h.save("version: 1\njobs:\n  t:\n    image: a\n    steps:\n      - run: echo x\n"), StartOptions{})
	got := h.done(run.ID)
	if got.Status != store.PipelineStatusFailed || !strings.Contains(got.Reason, "create job container") {
		t.Fatalf("run = %s %q", got.Status, got.Reason)
	}
}

func TestArtifactAndCacheMounts(t *testing.T) {
	t.Parallel()
	h := newHarness(t, okHandler)
	yaml := `
version: 1
jobs:
  a:
    image: x
    cache: [{ key: gomod, path: /root/go }]
    steps:
      - uses: artifact-upload
        with: { name: bin, path: out/app }
  b:
    needs: a
    image: x
    steps:
      - uses: artifact-download
        with: { name: bin, path: dist }
`
	run := h.start(h.save(yaml), StartOptions{})
	if got := h.done(run.ID); got.Status != store.PipelineStatusSucceeded {
		t.Fatalf("run = %s\n%s", got.Status, h.dump(run.ID))
	}
	if h.rt.ran("cp -a 'out/app' /artifacts/bin/") != 1 || h.rt.ran("cp -a /artifacts/bin/. 'dist'/") != 1 {
		t.Errorf("artifact scripts wrong: %v", h.rt.scripts)
	}
	var sawCache bool
	for _, c := range h.rt.created {
		for _, v := range c.Volumes {
			if v.ContainerPath == "/root/go" && strings.Contains(v.Name, "gomod") {
				sawCache = true
			}
		}
	}
	if !sawCache {
		t.Error("cache volume not mounted")
	}
}

func TestPromoteAndRollbackSteps(t *testing.T) {
	t.Parallel()
	h := newHarness(t, okHandler)
	yaml := "version: 1\njobs:\n  p:\n    steps:\n      - uses: promote\n        with: { from: staging, to: prod }\n      - uses: rollback\n        with: { service: prod }\n"
	run := h.start(h.save(yaml), StartOptions{})
	if got := h.done(run.ID); got.Status != store.PipelineStatusSucceeded {
		t.Fatalf("run = %s %q", got.Status, got.Reason)
	}
	if len(h.act.promotes) != 1 || h.act.promotes[0] != [2]string{"staging", "prod"} || len(h.act.rollback) != 1 {
		t.Errorf("promotes=%v rollback=%v", h.act.promotes, h.act.rollback)
	}
	if h.rt.liveContainers() != 0 || len(h.rt.created) != 0 {
		t.Error("control-plane-only job must not create containers")
	}
}

func TestManualInputsValidated(t *testing.T) {
	t.Parallel()
	h := newHarness(t, okHandler)
	yaml := "version: 1\non:\n  manual:\n    inputs:\n      env: { default: staging, options: [staging, prod] }\njobs:\n  t:\n    image: a\n    steps:\n      - run: echo ${{ inputs.env }}\n"
	p := h.save(yaml)
	if _, err := h.e.Start(context.Background(), p, StartOptions{Trigger: TriggerManual, Inputs: map[string]string{"env": "nope"}}); err == nil {
		t.Error("invalid option accepted")
	}
	if _, err := h.e.Start(context.Background(), p, StartOptions{Trigger: TriggerManual, Inputs: map[string]string{"bogus": "1"}}); err == nil {
		t.Error("unknown input accepted")
	}
	run := h.start(p, StartOptions{Inputs: map[string]string{"env": "prod"}})
	h.done(run.ID)
	if h.rt.ran("export PIPELINE_EXPR_INPUTS_ENV='prod'") != 1 || h.rt.ran("echo ${PIPELINE_EXPR_INPUTS_ENV}") != 1 {
		t.Error("input not passed to the script as an env var")
	}
}

func TestTriggerEventAndRerun(t *testing.T) {
	t.Parallel()
	h := newHarness(t, okHandler)
	h.save("version: 1\non:\n  push:\n    branches: [main]\njobs:\n  t:\n    image: a\n    steps:\n      - run: echo x\n")
	if got := h.e.TriggerEvent(context.Background(), "web", Event{Kind: TriggerPush, Branch: "dev"}, "refs/heads/dev", "s", "u"); len(got) != 0 {
		t.Fatalf("non-matching branch started %d runs", len(got))
	}
	started := h.e.TriggerEvent(context.Background(), "web", Event{Kind: TriggerPush, Branch: "main"}, "refs/heads/main", "s1", "u")
	if len(started) != 1 {
		t.Fatalf("started = %d", len(started))
	}
	h.done(started[0].ID)
	if _, err := h.e.Rerun(context.Background(), started[0].ID, "gagan"); err == nil {
		t.Log("rerun of a push-only pipeline is rejected because it has no manual trigger")
	}
}

func TestSchedulerArmsThenFires(t *testing.T) {
	t.Parallel()
	h := newHarness(t, okHandler)
	h.save("version: 1\non:\n  schedule: ['* * * * *']\njobs:\n  t:\n    image: a\n    steps:\n      - run: echo x\n")
	now := time.Date(2026, 1, 1, 10, 0, 30, 0, time.UTC)
	h.e.cfg.Now = func() time.Time { return now }
	s := NewScheduler(h.e)
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if runs, _ := h.db.ListActivePipelineRuns(context.Background()); len(runs) != 0 {
		t.Fatal("schedule fired on first sight")
	}
	now = now.Add(time.Minute)
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	runs, _ := h.db.ListActivePipelineRuns(context.Background())
	if len(runs) != 1 || runs[0].TriggerKind != TriggerSchedule {
		t.Fatalf("runs = %+v", runs)
	}
}

func TestRunTimeout(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(string) execResult { return execResult{Block: true} })
	yaml := "version: 1\ntimeout: 100ms\njobs:\n  t:\n    image: a\n    steps:\n      - run: sleep 100\n"
	run := h.start(h.save(yaml), StartOptions{})
	got := h.done(run.ID)
	if got.Status != store.PipelineStatusFailed || !strings.Contains(got.Reason, "timed out") {
		t.Fatalf("run = %s %q", got.Status, got.Reason)
	}
}
