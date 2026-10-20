package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testPolicyDocument = `{"Statement":[{"Effect":"Allow","Action":["read"],"Resource":["app:web"]}]}`

func TestHandleCreatePolicy(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"name":"web-readers","description":"read access to web app","document":` + testPolicyDocument + `}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/iam/policies", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got policyResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Name != "web-readers" || got.ID == "" {
		t.Errorf("got %+v", got)
	}
}

func TestHandleCreatePolicy_InvalidDocument(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"name":"bad","description":"","document":{"Statement":[]}}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/iam/policies", body))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleCreatePolicy_DuplicateName(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"name":"dup","description":"","document":` + testPolicyDocument + `}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/iam/policies", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("first create status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec2 := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec2, authedRequest(t, cookie, http.MethodPost, "/api/v1/iam/policies", body))
	if rec2.Code != http.StatusConflict {
		t.Errorf("second create status = %d, want %d, body = %s", rec2.Code, http.StatusConflict, rec2.Body.String())
	}
}

func TestHandleGetListUpdateDeletePolicy(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	createBody := `{"name":"lifecycle","description":"orig","document":` + testPolicyDocument + `}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/iam/policies", createBody))
	var created policyResource
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	getRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(getRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/iam/policies/"+created.ID, ""))
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", getRec.Code, getRec.Body.String())
	}

	listRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(listRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/iam/policies", ""))
	var list []policyResource
	if err := json.Unmarshal(listRec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list len = %d, want 1", len(list))
	}

	newDoc := `{"Statement":[{"Effect":"Deny","Action":["write"],"Resource":["*"]}]}`
	updateBody := `{"name":"lifecycle-renamed","description":"updated","document":` + newDoc + `}`
	updateRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(updateRec, authedRequest(t, cookie, http.MethodPut, "/api/v1/iam/policies/"+created.ID, updateBody))
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", updateRec.Code, updateRec.Body.String())
	}
	var updated policyResource
	if err := json.Unmarshal(updateRec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode update: %v", err)
	}
	if updated.Name != "lifecycle-renamed" || updated.Description != "updated" {
		t.Errorf("got %+v after update", updated)
	}

	deleteRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(deleteRec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/iam/policies/"+created.ID, ""))
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", deleteRec.Code, deleteRec.Body.String())
	}

	getAfterDeleteRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(getAfterDeleteRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/iam/policies/"+created.ID, ""))
	if getAfterDeleteRec.Code != http.StatusNotFound {
		t.Errorf("get after delete status = %d, want %d", getAfterDeleteRec.Code, http.StatusNotFound)
	}
}

func TestHandleGetPolicy_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/iam/policies/missing", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleAttachAndDetachAndListPolicyAttachments(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	createBody := `{"name":"attach-test","description":"","document":` + testPolicyDocument + `}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/iam/policies", createBody))
	var created policyResource
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	attachRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(attachRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/iam/policies/"+created.ID+"/attachments", `{"principal_type":"user","principal_id":"user_1"}`))
	if attachRec.Code != http.StatusNoContent {
		t.Fatalf("attach status = %d, body = %s", attachRec.Code, attachRec.Body.String())
	}

	listRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(listRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/iam/policies/"+created.ID+"/attachments", ""))
	var attachments []policyAttachmentResource
	if err := json.Unmarshal(listRec.Body.Bytes(), &attachments); err != nil {
		t.Fatalf("decode attachments: %v", err)
	}
	if len(attachments) != 1 || attachments[0].PrincipalID != "user_1" {
		t.Fatalf("attachments = %+v", attachments)
	}

	detachRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(detachRec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/iam/policies/"+created.ID+"/attachments/user/user_1", ""))
	if detachRec.Code != http.StatusNoContent {
		t.Fatalf("detach status = %d, body = %s", detachRec.Code, detachRec.Body.String())
	}

	listAfterRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(listAfterRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/iam/policies/"+created.ID+"/attachments", ""))
	var afterAttachments []policyAttachmentResource
	if err := json.Unmarshal(listAfterRec.Body.Bytes(), &afterAttachments); err != nil {
		t.Fatalf("decode attachments after detach: %v", err)
	}
	if len(afterAttachments) != 0 {
		t.Errorf("attachments after detach = %+v, want empty", afterAttachments)
	}
}

func TestHandleAttachPolicy_InvalidPrincipalType(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	createBody := `{"name":"bad-principal","description":"","document":` + testPolicyDocument + `}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/iam/policies", createBody))
	var created policyResource
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	attachRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(attachRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/iam/policies/"+created.ID+"/attachments", `{"principal_type":"robot","principal_id":"x"}`))
	if attachRec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d, body = %s", attachRec.Code, http.StatusBadRequest, attachRec.Body.String())
	}
}

func TestIAMPolicyRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/iam/policies", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
