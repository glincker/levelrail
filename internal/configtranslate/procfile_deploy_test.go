package configtranslate

import "testing"

func codes(r Result) map[string]bool {
	m := map[string]bool{}
	for _, f := range r.Findings {
		m[f.Code] = true
	}
	return m
}

func TestProcfileReleaseHookOnWeb(t *testing.T) {
	res := Translate(map[string][]byte{"Procfile": []byte("web: serve\nrelease: migrate\n")}, ForDeploy(false))
	h := res.Fragment.Services["web"].Hooks
	if h == nil || h.PreDeploy != "migrate" {
		t.Fatalf("hooks = %+v", h)
	}
	if !codes(res)["heroku.release.predeploy"] {
		t.Error("expected predeploy finding")
	}
}

func TestProcfileReleaseWithoutWebAtDeploy(t *testing.T) {
	files := map[string][]byte{"Procfile": []byte("worker: w\nrelease: r\n")}
	res := Translate(files, ForDeploy(false))
	if h := res.Fragment.Services["worker"].Hooks; h == nil || h.PreDeploy != "r" {
		t.Fatalf("worker hooks = %+v", h)
	}
	if codes(res)["heroku.release.no-web"] {
		t.Error("deploy mode must attach the hook, not drop it")
	}
	if !codes(Translate(files))["heroku.release.no-web"] {
		t.Error("import mode keeps the no-web finding")
	}
}

func TestProcfileDockerfileOverridesCommand(t *testing.T) {
	files := map[string][]byte{
		"Procfile": []byte("web: serve --port 80\nworker: run-jobs\n"),
		"app.json": []byte(`{"buildpacks":[{"url":"https://example.com/custom"}]}`),
	}
	res := Translate(files, ForDeploy(true))
	for _, name := range []string{"web", "worker"} {
		svc := res.Fragment.Services[name]
		if svc.Build.Type != "dockerfile" {
			t.Errorf("%s build = %q", name, svc.Build.Type)
		}
		if len(svc.Command) != 3 || svc.Command[0] != "sh" {
			t.Errorf("%s command = %v", name, svc.Command)
		}
	}
	c := codes(res)
	if !c["heroku.dockerfile.command"] || !c["heroku.buildpack.ignored"] || c["heroku.buildpack.unsupported"] {
		t.Errorf("findings = %v", c)
	}
}

func TestProcfileWithoutDockerfileUsesRailpack(t *testing.T) {
	res := Translate(map[string][]byte{"Procfile": []byte("web: serve\n")}, ForDeploy(false))
	if got := res.Fragment.Services["web"].Build.Type; got != "railpack" {
		t.Errorf("build = %q", got)
	}
}

func TestProcfileAppJSONKeys(t *testing.T) {
	files := map[string][]byte{
		"Procfile": []byte("web: serve\nworker: run\nrelease: migrate\n"),
		"app.json": []byte(`{
  "healthchecks": {"web": [{"type": "startup", "path": "/up", "attempts": 3}, {"type": "liveness", "path": "/live"}], "ghost": [{"path": "/x"}]},
  "cron": [{"command": "rake tidy", "schedule": "0 3 * * *"}, {"command": "", "schedule": "* * * * *"}],
  "scripts": {"dokku": {"predeploy": "prep", "postdeploy": "warm", "other": "x"}},
  "formation": {"web": {"quantity": 3}, "ghost": {"quantity": 1}},
  "environments": {"test": {}}
}`),
	}
	res := Translate(files, ForDeploy(false))
	web := res.Fragment.Services["web"]
	if web.Health == nil || web.Health.Readiness.Path != "/up" || web.Health.Liveness.Path != "/live" {
		t.Errorf("health = %+v", web.Health)
	}
	if web.Replicas != 3 {
		t.Errorf("replicas = %d", web.Replicas)
	}
	if web.Hooks == nil || web.Hooks.PreDeploy != "migrate && prep" || web.Hooks.PostDeploy != "warm" {
		t.Errorf("hooks = %+v", web.Hooks)
	}
	if len(web.Jobs) != 1 || web.Jobs[0].Schedule != "0 3 * * *" {
		t.Errorf("jobs = %+v", web.Jobs)
	}
	c := codes(res)
	for _, want := range []string{
		"heroku.healthcheck.option", "heroku.healthcheck.unknown", "heroku.formation.unknown",
		"heroku.cron.invalid", "heroku.cron.job", "heroku.appjson.key.unsupported", "heroku.script.dokku.predeploy",
		"heroku.script.unsupported",
	} {
		if !c[want] {
			t.Errorf("missing finding %s in %v", want, c)
		}
	}
}

func TestProcfileCronParsesAsSpec(t *testing.T) {
	files := map[string][]byte{"Procfile": []byte("web: serve\n"), "app.json": []byte(`{"cron":[{"command":"x","schedule":"*/5 * * * *"}]}`)}
	res := Translate(files, ForDeploy(false))
	if _, err := res.Fragment.YAML(); err != nil {
		t.Fatal(err)
	}
}
