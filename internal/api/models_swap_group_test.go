package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestModels_CreateWithSwapGroup(t *testing.T) {
	rt, db := newModelsTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := doModels(t, rt, cookie, http.MethodPost, "/api/v1/models",
		`{"name":"a","engine":"ollama","model":"llama3.1:8b","swap_group":"gpu0"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var created createModelResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.SwapGroup != "gpu0" {
		t.Errorf("SwapGroup = %q, want gpu0", created.SwapGroup)
	}

	rec = doModels(t, rt, cookie, http.MethodPost, "/api/v1/models",
		`{"name":"bad","engine":"ollama","model":"llama3.1:8b","swap_group":"Not Valid!"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad swap_group status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestModels_SetSwapGroup_AndSharesGPUWith(t *testing.T) {
	rt, db := newModelsTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	for _, body := range []string{
		`{"name":"a","engine":"ollama","model":"llama3.1:8b"}`,
		`{"name":"b","engine":"ollama","model":"llama3.1:8b"}`,
	} {
		if rec := doModels(t, rt, cookie, http.MethodPost, "/api/v1/models", body); rec.Code != http.StatusCreated {
			t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
		}
	}

	rec := doModels(t, rt, cookie, http.MethodPut, "/api/v1/models/a/swap-group", `{"swap_group":"gpu0"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("set swap group a status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rec = doModels(t, rt, cookie, http.MethodPut, "/api/v1/models/b/swap-group", `{"swap_group":"gpu0"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("set swap group b status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = doModels(t, rt, cookie, http.MethodGet, "/api/v1/models/a", "")
	var got modelResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.SwapGroup != "gpu0" {
		t.Errorf("SwapGroup = %q, want gpu0", got.SwapGroup)
	}
	if len(got.SharesGPUWith) != 1 || got.SharesGPUWith[0] != "b" {
		t.Errorf("SharesGPUWith = %v, want [b]", got.SharesGPUWith)
	}

	rec = doModels(t, rt, cookie, http.MethodPut, "/api/v1/models/a/swap-group", `{"swap_group":""}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("clear swap group status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rec = doModels(t, rt, cookie, http.MethodGet, "/api/v1/models/a", "")
	var afterClear modelResource
	if err := json.Unmarshal(rec.Body.Bytes(), &afterClear); err != nil {
		t.Fatal(err)
	}
	if afterClear.SwapGroup != "" || len(afterClear.SharesGPUWith) != 0 {
		t.Errorf("after clear got = %+v, want no group and no peers", afterClear)
	}
}

func TestModels_SetSwapGroup_InvalidFormat(t *testing.T) {
	rt, db := newModelsTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := doModels(t, rt, cookie, http.MethodPost, "/api/v1/models", `{"name":"a","engine":"ollama","model":"llama3.1:8b"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = doModels(t, rt, cookie, http.MethodPut, "/api/v1/models/a/swap-group", `{"swap_group":"Not Valid!"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}
