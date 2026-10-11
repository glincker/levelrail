package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/GLINCKER/levelrail/internal/appwizard"
)

// TestAnalyzeImport_WizardFinalRequestsDeployAnalyzedBuild replays the
// wizard's final request sequence with values taken from a real analysis
// response, through the real handlers, and checks the build gets the
// analyzed settings.
func TestAnalyzeImport_WizardFinalRequestsDeployAnalyzedBuild(t *testing.T) {
	fb := newFakeBuilder("shop:main", nil)
	fetch := newFakeFetch(t.TempDir(), nil)
	rt, db := newTestRouterWithBuilder(t, fb, fetch)
	cookie := loginTestSession(t, rt, db)
	files := map[string]string{
		"turbo.json":             "{}",
		"package.json":           `{"workspaces":["apps/*"]}`,
		"apps/web/package.json":  `{"scripts":{"start":"next start"},"dependencies":{"next":"14","pg":"8"}}`,
		"apps/web/Dockerfile":    "FROM node:22\nRUN npx turbo prune web --docker\nEXPOSE 3000\n",
		"apps/web/.env.example":  "DATABASE_URL=\nSESSION_SECRET=changeme\n",
		"apps/docs/package.json": "{}",
	}
	useFakeLayout(rt, &fakeLayoutSource{files: files})

	rec := layoutReq(t, rt, cookie, http.MethodPost, "/api/v1/imports/analyze", `{"repo_url":"https://github.com/acme/mono","ref":"main"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("analyze status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var a appwizard.Analysis
	if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	if len(a.Databases) != 1 || a.Build.DockerfilePath == "" {
		t.Fatalf("analysis = %+v", a)
	}
	dbName := a.Databases[0].SuggestedName

	steps := []struct {
		method, path, body string
		want               int
	}{
		{http.MethodPost, "/api/v1/databases", `{"name":"` + dbName + `","engine":"` + a.Databases[0].Engine + `","version":"` + a.Databases[0].Version + `"}`, http.StatusCreated},
		{http.MethodPost, "/api/v1/apps", `{"name":"` + a.Name.Value + `","image":"` + a.Name.Value + `:pending","port":` + strconv.Itoa(a.Port.Value) + `,"health":{"readiness":{"path":"` + a.Health.Value + `"}},"resources":{"memory_bytes":` + strconv.FormatInt(a.Memory.Bytes, 10) + `},"env":{"LOG_LEVEL":"info"}}`, http.StatusCreated},
		{http.MethodPost, "/api/v1/apps/" + a.Name.Value + "/connections", `{"database":"` + dbName + `","env_var":"` + a.Databases[0].EnvVar + `","field":"url"}`, http.StatusOK},
	}
	for _, s := range steps {
		r := layoutReq(t, rt, cookie, s.method, s.path, s.body)
		if r.Code != s.want {
			t.Fatalf("%s %s = %d, want %d, body = %s", s.method, s.path, r.Code, s.want, r.Body.String())
		}
	}

	buildBody := `{"repo_url":"https://github.com/acme/mono","ref":"` + a.Branch + `","build":{"type":"` + a.Build.Type + `","path":"` + a.Build.DockerfilePath + `","base_directory":"` + a.Build.BaseDirectory + `"}}`
	br := layoutReq(t, rt, cookie, http.MethodPost, "/api/v1/apps/"+a.Name.Value+"/builds", buildBody)
	if br.Code != http.StatusAccepted {
		t.Fatalf("build status = %d, body = %s", br.Code, br.Body.String())
	}
	got := fb.awaitCall(t)
	if got.ServiceName != a.Name.Value || got.Service.Build.Type != "dockerfile" || got.Service.Build.Path != a.Build.DockerfilePath {
		t.Fatalf("builder saw %+v", got)
	}
}
