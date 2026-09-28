package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_NodeCertCalls(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/nodes/nd_1/reenroll-token":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(NodeReenrollTokenResponse{Token: "tok", NodeID: "nd_1"})
		case "/api/v1/nodes/nd_1/revoke-cert":
			_ = json.NewEncoder(w).Encode(NodeResource{ID: "nd_1", Cert: &NodeCertResource{State: "revoked"}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	client := NewClient(srv.URL, "test-token")

	tok, err := client.CreateNodeReenrollToken(context.Background(), "nd_1")
	if err != nil || tok.Token != "tok" || gotMethod != http.MethodPost || gotPath != "/api/v1/nodes/nd_1/reenroll-token" {
		t.Fatalf("CreateNodeReenrollToken() = %+v, %v via %s %s", tok, err, gotMethod, gotPath)
	}
	node, err := client.RevokeNodeCert(context.Background(), "nd_1")
	if err != nil || node.Cert == nil || node.Cert.State != "revoked" || gotPath != "/api/v1/nodes/nd_1/revoke-cert" {
		t.Fatalf("RevokeNodeCert() = %+v, %v via %s", node, err, gotPath)
	}
}
