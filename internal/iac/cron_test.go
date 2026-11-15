package iac

import (
	"strings"
	"testing"
)

const cronAppDoc = "version: 1\nkind: App\nmetadata: {name: web}\nspec:\n  service:\n    build: {type: image, image: x}\n    port: 80\n---\n"

func TestCronAndAlertInvalidFilesFailWithStableCodes(t *testing.T) {
	cases := []struct{ name, doc, code string }{
		{"bad schedule", "version: 1\nkind: CronJob\nmetadata: {name: tick}\nspec: {app: web, schedule: \"every day\", command: [a]}\n", CodeCronScheduleInvalid},
		{"bad timezone", "version: 1\nkind: CronJob\nmetadata: {name: tick}\nspec: {app: web, schedule: \"0 * * * *\", timezone: Mars/Base, command: [a]}\n", CodeCronJobInvalid},
		{"bad name", "version: 1\nkind: CronJob\nmetadata: {name: Tick}\nspec: {app: web, schedule: \"0 * * * *\", command: [a]}\n", CodeCronJobInvalid},
		{"http and command", "version: 1\nkind: CronJob\nmetadata: {name: t}\nspec: {app: web, schedule: \"0 * * * *\", command: [a], http: {path: /x}}\n", CodeCronHTTPInvalid},
		{"http relative path", "version: 1\nkind: CronJob\nmetadata: {name: t}\nspec: {app: web, schedule: \"0 * * * *\", http: {path: x}}\n", CodeCronHTTPInvalid},
		{"http scheme relative path", "version: 1\nkind: CronJob\nmetadata: {name: t}\nspec: {app: web, schedule: \"0 * * * *\", http: {path: //evil.test/x}}\n", CodeCronHTTPInvalid},
		{"http bad method", "version: 1\nkind: CronJob\nmetadata: {name: t}\nspec: {app: web, schedule: \"0 * * * *\", http: {path: /x, method: TRACE}}\n", CodeCronHTTPInvalid},
		{"http header both value and secret", "version: 1\nkind: CronJob\nmetadata: {name: t}\nspec: {app: web, schedule: \"0 * * * *\", http: {path: /x, headers: [{name: A, value: b, secretRef: C}]}}\n", CodeCronHTTPInvalid},
		{"threshold without metric", "version: 1\nkind: AlertRule\nmetadata: {name: r}\nspec: {app: web, type: threshold, comparator: \">\", threshold: 1}\n", CodeAlertRuleInvalid},
		{"threshold bad comparator", "version: 1\nkind: AlertRule\nmetadata: {name: r}\nspec: {app: web, type: threshold, metric: cpu, comparator: \"=\", threshold: 1}\n", CodeAlertRuleInvalid},
	}
	for _, c := range cases {
		docs, issues := ParseDocuments([]Source{{Name: "t.yaml", Data: []byte(c.doc)}})
		if len(issues) == 0 {
			_, issues = Build(docs, Options{})
		}
		found := false
		for _, is := range issues {
			found = found || is.Code == c.code
		}
		if !found {
			t.Errorf("%s: want code %s, got %+v", c.name, c.code, issues)
		}
	}
}

func TestCronJobPlanApplyExportAndPrune(t *testing.T) {
	src := cronAppDoc + "version: 1\nkind: CronJob\nmetadata: {name: nightly}\nspec: {app: web, schedule: \"0 3 * * *\", command: [node, n.js], retries: 2}\n"
	f := newFake()
	opts := Options{Source: "git"}
	if out := applyAll(t, f, src, opts); !out.OK() {
		t.Fatalf("apply = %+v", out.Results)
	}
	if _, ok := f.jobs["web"]["nightly"]; !ok {
		t.Fatalf("job not created: %+v", f.jobs)
	}
	if p := planFor(t, f, src, opts); p.Summary.Create+p.Summary.Update+p.Summary.Delete != 0 {
		t.Fatalf("second plan is not a no-op: %+v", p.Changes)
	}
	changed := strings.Replace(src, "retries: 2", "retries: 3", 1)
	if p := planFor(t, f, changed, opts); p.Summary.Update != 1 {
		t.Fatalf("changed job should plan one update: %+v", p.Changes)
	}
	conflict := strings.Replace(src, "port: 80\n", "port: 80\n    jobs:\n      - {name: nightly, schedule: \"0 1 * * *\", command: [x]}\n", 1)
	if p := planFor(t, f, conflict, opts); p.Summary.Error == 0 {
		t.Fatalf("a job declared in the app and as a CronJob must be refused: %+v", p.Changes)
	}
}

func TestCronHTTPJobCreatesAndRoundTrips(t *testing.T) {
	src := cronAppDoc + "version: 1\nkind: CronJob\nmetadata: {name: warm}\nspec: {app: web, schedule: \"*/10 * * * *\", http: {path: /warm, method: post, headers: [{name: Authorization, secretRef: TOK, prefix: \"Bearer \"}]}}\n"
	f := newFake()
	opts := Options{Source: "git"}
	if out := applyAll(t, f, src, opts); !out.OK() {
		t.Fatalf("apply = %+v", out.Results)
	}
	body := f.jobs["web"]["warm"]
	h, ok := body["http"].(map[string]any)
	if body["mode"] != "http" || !ok || h["method"] != "POST" || h["path"] != "/warm" {
		t.Fatalf("stored job = %+v", body)
	}
	if out := applyAll(t, f, src, opts); !out.OK() {
		t.Fatalf("second apply = %+v", out.Results)
	}
}
