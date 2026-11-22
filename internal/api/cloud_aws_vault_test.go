package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAWSVaultKitEndpoint(t *testing.T) {
	rt := &Router{}
	body := `{"account_id":"123456789012","vault_principal_arn":"arn:aws:iam::999999999999:user/vault","capabilities":["inventory"],"vault_role":"lr"}`
	w := httptest.NewRecorder()
	rt.handleAWSVaultKit(w, httptest.NewRequest(http.MethodPost, "/api/v1/cloud-accounts/aws/vault-kit", strings.NewReader(body)))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "roles/lr") || !strings.Contains(w.Body.String(), "trust_policy_json") {
		t.Fatalf("vault kit = %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	rt.handleAWSVaultKit(w, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"account_id":"1","vault_principal_arn":"x"}`)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad kit = %d", w.Code)
	}
}
