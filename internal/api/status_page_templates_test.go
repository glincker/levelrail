package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestStatusTemplates_CRUD(t *testing.T) {
	rt, _, cookie := newStatusRouter(t, 100)
	const path = "/api/v1/status-page/incident-templates"

	rec := apiDo(t, rt, cookie, http.MethodGet, path, "")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("empty list = %d %s, want 200 []", rec.Code, rec.Body.String())
	}
	bad := []string{
		`{"name":"","title":"x"}`,
		`{"name":"n","title":""}`,
		`{"name":"n","title":"x","impact":"huge"}`,
		`{"name":"n","title":"x","status":"scheduled"}`,
	}
	for _, b := range bad {
		if rec := apiDo(t, rt, cookie, http.MethodPost, path, b); rec.Code != http.StatusBadRequest {
			t.Errorf("POST %s = %d, want 400", b, rec.Code)
		}
	}
	rec = apiDo(t, rt, cookie, http.MethodPost, path, `{"name":"DB slow","title":"Database is slow","impact":"minor","body":"We are looking into it."}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	var created statusTemplateResource
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.Status != "investigating" {
		t.Fatalf("created = %+v err %v, want default status investigating", created, err)
	}
	if rec := apiDo(t, rt, cookie, http.MethodPost, path, `{"name":"DB slow","title":"again"}`); rec.Code != http.StatusConflict {
		t.Errorf("duplicate name = %d, want 409", rec.Code)
	}
	rec = apiDo(t, rt, cookie, http.MethodPut, path+"/"+created.ID, `{"name":"DB slow","title":"Database is slow","status":"identified","impact":"major"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"identified"`) {
		t.Errorf("update = %d %s", rec.Code, rec.Body.String())
	}
	if rec := apiDo(t, rt, cookie, http.MethodPut, path+"/st_missing", `{"name":"a","title":"b"}`); rec.Code != http.StatusNotFound {
		t.Errorf("update missing = %d, want 404", rec.Code)
	}
	if rec := apiDo(t, rt, cookie, http.MethodDelete, path+"/"+created.ID, ""); rec.Code != http.StatusNoContent {
		t.Errorf("delete = %d, want 204", rec.Code)
	}
	if rec := apiDo(t, rt, cookie, http.MethodDelete, path+"/"+created.ID, ""); rec.Code != http.StatusNotFound {
		t.Errorf("delete again = %d, want 404", rec.Code)
	}
}
