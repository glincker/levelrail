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
