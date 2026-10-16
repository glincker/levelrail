package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/models"
	"github.com/GLINCKER/levelrail/internal/store"
)

func newCPUModelsRouter(t *testing.T) (*Router, *store.DB) {
	_, rt, db := newCPUModelsRouterSvc(t)
	return rt, db
}

func newCPUModelsRouterSvc(t *testing.T) (*models.Service, *Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	svc := models.NewService(db, nil, models.NewHostResolver("", nil), "")
	svc.SetHostFacts(func(context.Context, string) (models.HostFacts, bool) {
		return models.HostFacts{CPUs: 4, MemTotalBytes: 16 << 30, MemAvailBytes: 12 << 30}, true
	})
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return svc, NewRouter(logger, testBrand(), db, WithModels(svc)), db
}

func TestModelsCPU_RoutesRequireAuth(t *testing.T) {
	rt, _ := newCPUModelsRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/model-catalog"},
		{http.MethodPost, "/api/v1/models/chat/benchmark"},
	})
}

func TestModelsCPU_CatalogAndFit(t *testing.T) {
	rt, db := newCPUModelsRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := doModels(t, rt, cookie, http.MethodGet, "/api/v1/model-catalog", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("catalog status = %d: %s", rec.Code, rec.Body.String())
	}
	var view models.CatalogView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !view.HostKnown || len(view.Models) < 5 || view.Models[0].Tier != models.TierLight || !view.Models[0].Fit.Estimate {
		t.Fatalf("catalog = %+v", view)
	}

	rec = doModels(t, rt, cookie, http.MethodPost, "/api/v1/models/fit", `{"compute":"cpu","catalog_id":"granite-4.1-3b"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("fit status = %d: %s", rec.Code, rec.Body.String())
	}
	var fit models.CPUFitReport
	_ = json.Unmarshal(rec.Body.Bytes(), &fit)
	if fit.Verdict != models.CPUFitComfortable || !fit.LicenseCurated || fit.TokensPerSecHi <= 0 {
		t.Fatalf("fit = %+v", fit)
	}

	rec = doModels(t, rt, cookie, http.MethodPost, "/api/v1/models/fit", `{"compute":"cpu","model":"someone/cool-3B-GGUF:Q4_K_M"}`)
	_ = json.Unmarshal(rec.Body.Bytes(), &fit)
	if rec.Code != http.StatusOK || fit.LicenseCurated || fit.LicenseNotice == "" {
		t.Fatalf("custom fit = %d %+v", rec.Code, fit)
	}
}

func TestModelsCPU_CreateFlowAndLicenseAck(t *testing.T) {
	svc, rt, db := newCPUModelsRouterSvc(t)
	cookie := loginTestSession(t, rt, db)

	rec := doModels(t, rt, cookie, http.MethodPost, "/api/v1/models", `{"name":"local","catalog_id":"granite-4.0-1b"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	var created createModelResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.Compute != "cpu" || created.CPULimit != 2 || created.MemoryLimitMiB != 4096 || created.WeightsFile == "" || created.LicenseAck {
		t.Errorf("created = %+v", created.modelCPUResource)
	}

	body := `{"name":"custom","compute":"cpu","model":"someone/cool-3B-GGUF:Q4_K_M","weights_file":"cool.gguf"}`
	rec = doModels(t, rt, cookie, http.MethodPost, "/api/v1/models", body)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "license_ack_required") {
		t.Fatalf("custom without ack = %d: %s", rec.Code, rec.Body.String())
	}
	rec = doModels(t, rt, cookie, http.MethodPost, "/api/v1/models", strings.Replace(body, "}", `,"accept_license":true}`, 1))
	if rec.Code != http.StatusCreated {
		t.Fatalf("custom with ack = %d: %s", rec.Code, rec.Body.String())
	}

	rec = doModels(t, rt, cookie, http.MethodPost, "/api/v1/models", `{"name":"toobig","catalog_id":"gemma-4-e4b","memory_mib":3072}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "will not fit") {
		t.Fatalf("oversized = %d: %s", rec.Code, rec.Body.String())
	}

	rec = doModels(t, rt, cookie, http.MethodPost, "/api/v1/models/local/benchmark", "")
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("benchmark without a wired runner = %d: %s", rec.Code, rec.Body.String())
	}
	svc.SetBenchmark(models.LoadBenchConfig(), nil, nil)
	rec = doModels(t, rt, cookie, http.MethodPost, "/api/v1/models/local/benchmark", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("benchmark of a model that is not running = %d: %s", rec.Code, rec.Body.String())
	}
}
