package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/test/e2e/testenv"
)

const (
	e2eRTCAdminUsername = "e2e-rtc-admin"
	e2eRTCAdminPassword = "e2e-rtc-correct-horse" //nolint:gosec // test fixture credential, not a real secret
	e2eRTCImage         = "alpine:3.20"
)

// Timing knobs, so a slow host can stretch the run without editing the test.
const (
	envRTCReadyBudget  = "LEVELRAIL_E2E_RTC_READY_BUDGET"
	envRTCMigrateSleep = "LEVELRAIL_E2E_RTC_MIGRATE_SLEEP"
	envRTCBackoffBase  = "LEVELRAIL_E2E_RTC_BACKOFF_BASE"
	envRTCObserve      = "LEVELRAIL_E2E_RTC_OBSERVE"
)

func rtcDuration(t *testing.T, key string, def time.Duration) time.Duration {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		t.Fatalf("%s=%q: want a positive duration", key, raw)
	}
	return d
}

type rtcContainer struct {
	ID         string
	Name       string
	Running    bool
	ExitCode   int
	StartedAt  time.Time
	FinishedAt time.Time
}

func (c rtcContainer) started() bool { return !c.StartedAt.IsZero() }

func (c rtcContainer) String() string {
	return fmt.Sprintf("%s running=%t exit=%d started=%s finished=%s", c.Name, c.Running, c.ExitCode,
		c.StartedAt.Format(time.RFC3339Nano), c.FinishedAt.Format(time.RFC3339Nano))
}

// rtcStack drives one compose app's service controllers the way the engine
// does: one pass per service, conditions persisted where the API reads them.
type rtcStack struct {
	t        *testing.T
	app      string
	services []string
	ctrls    map[string]*application.Controller
	store    *store.DB
	rt       docker.Runtime
	cli      *dockerclient.Client
	opts     func() []application.Option
	last     map[string]reconcile.Condition
}

func (s *rtcStack) svc(name string) string { return s.app + "-" + name }

// boot builds fresh controllers, the same as a control plane restart would.
func (s *rtcStack) boot() {
	s.ctrls = make(map[string]*application.Controller, len(s.services))
	for _, name := range s.services {
		s.ctrls[name] = application.New(s.svc(name), s.store, s.rt, s.opts()...)
	}
}

func (s *rtcStack) pass(ctx context.Context) {
	s.t.Helper()
	if s.last == nil {
		s.last = map[string]reconcile.Condition{}
	}
	for _, name := range s.services {
		rctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		res, err := s.ctrls[name].Reconcile(rctx)
		cancel()
		if len(res.Conditions) > 0 {
			if uerr := s.store.UpsertConditions(ctx, application.ControllerName(s.svc(name)), res.Conditions); uerr != nil {
				s.t.Fatalf("UpsertConditions(%s): %v", name, uerr)
			}
			s.last[name] = res.Conditions[0]
		}
		if err != nil {
			s.t.Logf("  %s: reconcile err = %v", name, err)
		}
	}
}

func (s *rtcStack) cond(name string) reconcile.Condition { return s.last[name] }

func (s *rtcStack) containers(name string) []rtcContainer {
	s.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	prefix := s.svc(name) + "-"
	list, err := s.cli.ContainerList(ctx, container.ListOptions{All: true, Filters: filters.NewArgs(filters.Arg("name", prefix))})
	if err != nil {
		s.t.Fatalf("ContainerList(%s): %v", prefix, err)
	}
	var out []rtcContainer
	for _, c := range list {
		if len(c.Names) == 0 || !strings.HasPrefix(strings.TrimPrefix(c.Names[0], "/"), prefix) {
			continue
		}
		info, err := s.cli.ContainerInspect(ctx, c.ID)
		if err != nil {
			s.t.Fatalf("ContainerInspect(%s): %v", c.ID, err)
		}
		rc := rtcContainer{ID: c.ID, Name: strings.TrimPrefix(info.Name, "/")}
		if info.State != nil {
			rc.Running = info.State.Running
			rc.ExitCode = info.State.ExitCode
			rc.StartedAt = rtcTime(info.State.StartedAt)
			rc.FinishedAt = rtcTime(info.State.FinishedAt)
		}
		out = append(out, rc)
	}
	return out
}

func (s *rtcStack) one(name string) (rtcContainer, bool) {
	s.t.Helper()
	list := s.containers(name)
	if len(list) > 1 {
		s.t.Fatalf("%s: %d containers, want at most 1: %v", name, len(list), list)
	}
	if len(list) == 0 {
		return rtcContainer{}, false
	}
	return list[0], true
}

func (s *rtcStack) cleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	for _, name := range s.services {
		cleanupContainers(ctx, s.t, s.rt, s.svc(name))
	}
	if err := s.rt.RemoveNetwork(ctx, application.NetworkName("", s.app)); err != nil {
		s.t.Logf("cleanup: RemoveNetwork(%q) error = %v", s.app, err)
	}
}

// Docker reports a never-started container's times as the zero year.
func rtcTime(raw string) time.Time {
	ts, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil || ts.Year() <= 1 {
		return time.Time{}
	}
	return ts
}

func rtcOutcome(t *testing.T, client *http.Client, baseURL, svc string) (int, string, string) {
	t.Helper()
	status, body := requestJSON(t, client, http.MethodGet, baseURL+"/api/v1/apps/"+svc+"/deploys/latest", "")
	if status != http.StatusOK {
		return status, "", string(body)
	}
	var got struct {
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode deploy %s: %v, body = %s", svc, err, body)
	}
	return status, got.Outcome, string(body)
}

func rtcPutEnv(t *testing.T, client *http.Client, baseURL, svc, env string) {
	t.Helper()
	status, body := requestJSON(t, client, http.MethodGet, baseURL+"/api/v1/apps/"+svc, "")
	if status != http.StatusOK {
		t.Fatalf("GET %s: status = %d, body = %s", svc, status, body)
	}
	var detail map[string]json.RawMessage
	if err := json.Unmarshal(body, &detail); err != nil {
		t.Fatalf("decode GET %s: %v", svc, err)
	}
	detail["env"] = json.RawMessage(env)
	edited, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("encode edit: %v", err)
	}
	status, body = requestJSON(t, client, http.MethodPut, baseURL+"/api/v1/apps/"+svc, string(edited))
	if status != http.StatusOK {
		t.Fatalf("PUT %s: status = %d, body = %s", svc, status, body)
	}
}

func rtcCompose(migrateRestart, migrateCmd string, withApp bool) string {
	restart := ""
	if migrateRestart != "" {
		restart = "    restart: \"" + migrateRestart + "\"\n"
	}
	out := "services:\n  migrate:\n    image: " + e2eRTCImage + "\n" + restart +
		"    command: [\"sh\", \"-c\", \"" + migrateCmd + "\"]\n"
	if withApp {
		out += "  app:\n    image: " + e2eRTCImage + "\n    command: [\"sleep\", \"3600\"]\n" +
			"    depends_on:\n      migrate:\n        condition: service_completed_successfully\n"
	}
	return out
}

// TestRunToCompletion_Live drives one-shot compose services through the real
// API and reconcile controllers on real Docker.
func TestRunToCompletion_Live(t *testing.T) {
	testenv.RequireFullLive(t)
	env := newLiveBuildEnv(t)
	rt := env.Runtime
	if _, ok := rt.(docker.ExitStateInspector); !ok {
		t.Fatal("live runtime hides docker.ExitStateInspector, so one-shots can never complete")
	}

	readyBudget := rtcDuration(t, envRTCReadyBudget, 4*time.Second)
	migrateSleep := rtcDuration(t, envRTCMigrateSleep, 12*time.Second)
	backoffBase := rtcDuration(t, envRTCBackoffBase, 2*time.Second)
	observe := rtcDuration(t, envRTCObserve, 30*time.Second)
	if migrateSleep <= readyBudget+2*time.Second {
		t.Fatalf("%s (%s) must exceed %s (%s) by 2s to prove a one-shot outlives the ready budget", envRTCMigrateSleep, migrateSleep, envRTCReadyBudget, readyBudget)
	}

	svcStore := openLiveStore(t)
	b := &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}
	ts := newE2ETestServer(t, api.NewRouter(discardTestLogger(), b, svcStore))
	ctx := context.Background()
	if err := api.BootstrapAdmin(ctx, svcStore, e2eRTCAdminUsername, e2eRTCAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	client := loginE2EClient(t, ts.URL, e2eRTCAdminUsername, e2eRTCAdminPassword)

	newStack := func(app string, services ...string) *rtcStack {
		s := &rtcStack{t: t, app: app, services: services, store: svcStore, rt: rt, cli: env.DockerCli}
		s.opts = func() []application.Option {
			return []application.Option{
				application.WithReadyBudget(readyBudget),
				application.WithConditionReader(svcStore),
				application.WithRestartBackoff(application.NewRestartBackoff(backoffBase, 4*backoffBase, 10*time.Minute)),
			}
		}
		s.cleanup()
		t.Cleanup(s.cleanup)
		s.boot()
		return s
	}
	deploy := func(app, compose string) {
		t.Helper()
		status, body := requestJSON(t, client, http.MethodPost, ts.URL+"/api/v1/apps/"+app+"/compose", compose)
		if status != http.StatusOK {
			t.Fatalf("compose deploy %s: status = %d, body = %s", app, status, body)
		}
	}

	ok := newStack("rtcok", "migrate", "app")
	deploy(ok.app, rtcCompose("no", fmt.Sprintf("sleep %d; echo migrated", int(migrateSleep.Seconds())), true))
	var migrate rtcContainer

	t.Run("1 migration gates the app and is not killed", func(t *testing.T) {
		deadline := time.Now().Add(migrateSleep + 90*time.Second)
		pastBudget := false
		for {
			ok.pass(ctx)
			m, found := ok.one("migrate")
			a, appFound := ok.one("app")
			t.Logf("pass: migrate %s [%s]; app found=%t %s [%s]", m, ok.cond("migrate").Reason, appFound, a, ok.cond("app").Reason)
			if found && m.started() {
				if migrate.ID == "" {
					migrate = m
				}
				if m.ID != migrate.ID || !m.StartedAt.Equal(migrate.StartedAt) {
					t.Fatalf("migration re-run: was %s, now %s", migrate, m)
				}
				if m.Running && time.Since(m.StartedAt) > readyBudget+time.Second {
					pastBudget = true
				}
			}
			if appFound && a.started() && (!found || m.Running || m.FinishedAt.IsZero() || a.StartedAt.Before(m.FinishedAt)) {
				t.Fatalf("app started before the migration exited: app %s, migrate %s", a, m)
			}
			if c := ok.cond("app"); c.Status == reconcile.ConditionTrue {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("stack did not converge: migrate %+v, app %+v", ok.cond("migrate"), ok.cond("app"))
			}
			time.Sleep(time.Second)
		}
		if !pastBudget {
			t.Errorf("never saw the migration still running past the %s ready budget", readyBudget)
		}
		m, _ := ok.one("migrate")
		if m.Running || m.ExitCode != 0 {
			t.Fatalf("migrate = %s, want exited 0", m)
		}
		if c := ok.cond("migrate"); c.Status != reconcile.ConditionTrue || c.Reason != application.ReasonCompleted {
			t.Fatalf("migrate condition = %+v, want True %s", c, application.ReasonCompleted)
		}
		for range 3 {
			ok.pass(ctx)
			if m2, _ := ok.one("migrate"); m2.ID != migrate.ID || !m2.StartedAt.Equal(migrate.StartedAt) || m2.Running {
				t.Fatalf("migration re-run after completing: was %s, now %s", migrate, m2)
			}
			time.Sleep(time.Second)
		}
		for _, name := range []string{"migrate", "app"} {
			status, outcome, body := rtcOutcome(t, client, ts.URL, ok.svc(name))
			t.Logf("deploy outcome %s: %d %q", name, status, outcome)
			if status == http.StatusOK && outcome != api.OutcomeHealthy {
				t.Errorf("%s deploy outcome = %q, want %q: %s", name, outcome, api.OutcomeHealthy, body)
			}
		}
		t.Logf("migration ran once (%s), stayed up past the ready budget, completed, then the app started", migrate.StartedAt.Format(time.RFC3339))
	})
	if t.Failed() {
		return
	}

	t.Run("2 fresh controllers leave the completed one-shot alone", func(t *testing.T) {
		ok.boot()
		for range 3 {
			ok.pass(ctx)
			time.Sleep(time.Second)
		}
		m, _ := ok.one("migrate")
		if m.ID != migrate.ID || !m.StartedAt.Equal(migrate.StartedAt) || m.Running {
			t.Fatalf("after controller restart: migrate was %s, now %s", migrate, m)
		}
		if c := ok.cond("migrate"); c.Reason != application.ReasonCompleted || c.Status != reconcile.ConditionTrue {
			t.Fatalf("after controller restart: migrate condition = %+v", c)
		}
		if c := ok.cond("app"); c.Status != reconcile.ConditionTrue {
			t.Fatalf("after controller restart: app condition = %+v", c)
		}
	})

	t.Run("4 env edit via PUT keeps the restart policy", func(t *testing.T) {
		appBefore, _ := ok.one("app")
		rtcPutEnv(t, client, ts.URL, ok.svc("migrate"), `{"E2E_EDIT":"1"}`)
		rtcPutEnv(t, client, ts.URL, ok.svc("app"), `{"E2E_EDIT":"1"}`)
		for name, check := range map[string]func(store.DesiredService) bool{
			"migrate": func(d store.DesiredService) bool { return d.RunsToCompletion() },
			"app": func(d store.DesiredService) bool {
				return len(d.DependsOnCompleted()) == 1 && d.DependsOnCompleted()[0] == "migrate"
			},
		} {
			d, err := svcStore.GetDesiredService(ctx, ok.svc(name))
			if err != nil || d == nil {
				t.Fatalf("GetDesiredService(%s): %v", name, err)
			}
			if !check(*d) {
				t.Fatalf("after PUT %s lost its compose policy: health = %+v", name, d.Health)
			}
		}
		status, body := requestJSON(t, client, http.MethodPost, ts.URL+"/api/v1/apps/"+ok.svc("app")+"/restart", "")
		if status >= 300 {
			t.Fatalf("restart app: status = %d, body = %s", status, body)
		}
		deadline := time.Now().Add(90 * time.Second)
		for {
			ok.pass(ctx)
			a, found := ok.one("app")
			if found && a.Running && a.ID != appBefore.ID && ok.cond("app").Status == reconcile.ConditionTrue {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("app not redeployed after env edit: %s [%+v]", a, ok.cond("app"))
			}
			time.Sleep(time.Second)
		}
		ok.pass(ctx)
		m, _ := ok.one("migrate")
		if m.ID != migrate.ID || !m.StartedAt.Equal(migrate.StartedAt) || m.Running {
			t.Fatalf("env edit re-ran the migration: was %s, now %s", migrate, m)
		}
		t.Logf("env edits kept restart=no and the completed dependency; app replaced, migration not re-run")
	})

	t.Run("3 failing one-shot backs off and blocks the dependent", func(t *testing.T) {
		fail := newStack("rtcfail", "migrate", "app")
		deploy(fail.app, rtcCompose("on-failure", "sleep 1; echo boom; exit 1", true))
		starts := map[string]bool{}
		var failedReason, appReason string
		deadline := time.Now().Add(observe)
		for time.Now().Before(deadline) {
			fail.pass(ctx)
			for _, c := range fail.containers("migrate") {
				if c.started() {
					starts[c.ID+c.StartedAt.String()] = true
				}
			}
			if a := fail.containers("app"); len(a) > 0 {
				for _, c := range a {
					if c.started() {
						t.Fatalf("dependent started though its dependency fails: %s", c)
					}
				}
			}
			mc, ac := fail.cond("migrate"), fail.cond("app")
			_, passOutcome, _ := rtcOutcome(t, client, ts.URL, fail.svc("migrate"))
			t.Logf("pass: migrate [%s %s] app [%s %s] starts=%d outcome=%s", mc.Status, mc.Reason, ac.Status, ac.Reason, len(starts), passOutcome)
			if mc.Status == reconcile.ConditionTrue {
				t.Fatalf("failing one-shot reported Ready: %+v", mc)
			}
			if mc.Status == reconcile.ConditionFalse {
				failedReason = mc.Reason
			}
			if ac.Status == reconcile.ConditionFalse {
				appReason = ac.Reason
			}
			time.Sleep(time.Second)
		}
		if len(starts) < 2 {
			t.Errorf("failing one-shot started %d times in %s, want backoff restarts", len(starts), observe)
		}
		if failedReason == "" {
			t.Errorf("failing one-shot never reported a False condition with a reason")
		}
		if appReason != "DependencyFailed" {
			t.Errorf("dependent reason = %q, want DependencyFailed", appReason)
		}
		status, outcome, body := rtcOutcome(t, client, ts.URL, fail.svc("migrate"))
		t.Logf("deploy outcome migrate: %d %q", status, outcome)
		if status == http.StatusOK && outcome != api.OutcomeFailed {
			t.Errorf("failing one-shot deploy outcome = %q, want %q: %s", outcome, api.OutcomeFailed, body)
		}
		t.Logf("failing one-shot: %d starts under backoff, last reason %s, dependent %s", len(starts), failedReason, appReason)
	})

	t.Run("5 no explicit restart keeps restarting on exit 0", func(t *testing.T) {
		plain := newStack("rtcplain", "migrate")
		deploy(plain.app, rtcCompose("", "sleep 1; echo done", false))
		starts := map[string]bool{}
		deadline := time.Now().Add(observe)
		for time.Now().Before(deadline) {
			plain.pass(ctx)
			for _, c := range plain.containers("migrate") {
				if c.started() {
					starts[c.ID+c.StartedAt.String()] = true
				}
			}
			c := plain.cond("migrate")
			t.Logf("pass: [%s %s] starts=%d", c.Status, c.Reason, len(starts))
			if c.Reason == application.ReasonCompleted {
				t.Fatalf("service without an explicit restart reported %s", application.ReasonCompleted)
			}
			time.Sleep(time.Second)
		}
		if len(starts) < 2 {
			t.Errorf("service without restart started %d times in %s, want restarts under backoff", len(starts), observe)
		}
	})
}
