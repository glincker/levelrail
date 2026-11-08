package configtranslate

import "testing"

func TestChecksFileToReadiness(t *testing.T) {
	checks := "# gate\nWAIT=5\nTIMEOUT=7\nATTEMPTS=4\n/up welcome\n/second\n//host/x\nhttp://y\nPORT=1\n"
	res := Translate(map[string][]byte{"Procfile": []byte("web: serve\n"), "CHECKS": []byte(checks)}, ForDeploy(false))
	h := res.Fragment.Services["web"].Health
	if h == nil || h.Readiness == nil || h.Readiness.Path != "/up" || h.Readiness.Timeout != "7s" || h.Readiness.Failures != 4 {
		t.Fatalf("health = %+v", h)
	}
	c := codes(res)
	for _, want := range []string{"checks.wait", "checks.content", "checks.extra", "checks.host", "checks.unsupported", "checks.setting.unsupported"} {
		if !c[want] {
			t.Errorf("missing finding %s in %v", want, c)
		}
	}
	if c["heroku.health.none"] {
		t.Error("health.none must not fire when CHECKS gives a probe")
	}
}

func TestChecksAppJSONHealthchecksWin(t *testing.T) {
	files := map[string][]byte{
		"Procfile": []byte("web: serve\n"),
		"app.json": []byte(`{"healthchecks": {"web": [{"type": "startup", "path": "/from-appjson"}]}}`),
		"CHECKS":   []byte("/from-checks\n"),
	}
	res := Translate(files, ForDeploy(false))
	h := res.Fragment.Services["web"].Health
	if h == nil || h.Readiness.Path != "/from-appjson" || !codes(res)["checks.ignored"] {
		t.Fatalf("health = %+v codes = %v", h, codes(res))
	}
}
