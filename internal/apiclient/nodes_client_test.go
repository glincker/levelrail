package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClient_ListNodes(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]NodeResource{{ID: "nd_1", Name: "web-1", Status: "ready"}})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.ListNodes(context.Background())
	if err != nil {
		t.Fatalf("ListNodes() error = %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/api/v1/nodes" {
		t.Errorf("request = %s %s, want GET /api/v1/nodes", gotMethod, gotPath)
	}
	if len(got) != 1 || got[0].ID != "nd_1" {
		t.Errorf("ListNodes() = %+v, want one node nd_1", got)
	}
}

func TestClient_GetNode_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"node not found"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	_, err := client.GetNode(context.Background(), "nd_missing")
	if err == nil {
		t.Fatalf("GetNode() error = nil, want an error for a 404 response")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusNotFound)
	}
}

func TestClient_DeleteNode_Conflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"node \"nd_1\" still has 1 service(s) and 0 database(s) placed on it; drain it first via POST /api/v1/nodes/nd_1/drain"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	err := client.DeleteNode(context.Background(), "nd_1")
	if err == nil {
		t.Fatalf("DeleteNode() error = nil, want an error for a 409 response")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if apiErr.StatusCode != http.StatusConflict {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusConflict)
	}
	if apiErr.Message == "" {
		t.Errorf("Message is empty, want the server's drain-first message")
	}
}

func TestClient_SetNodeWorkloads(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody SetNodeWorkloadsRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(NodeResource{ID: "nd_1", AcceptsAppWorkloads: gotBody.AcceptsAppWorkloads, AcceptsBuildWorkloads: gotBody.AcceptsBuildWorkloads})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.SetNodeWorkloads(context.Background(), "nd_1", SetNodeWorkloadsRequest{AcceptsAppWorkloads: true, AcceptsBuildWorkloads: false})
	if err != nil {
		t.Fatalf("SetNodeWorkloads() error = %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/api/v1/nodes/nd_1/workloads" {
		t.Errorf("request = %s %s, want PUT /api/v1/nodes/nd_1/workloads", gotMethod, gotPath)
	}
	if !got.AcceptsAppWorkloads || got.AcceptsBuildWorkloads {
		t.Errorf("SetNodeWorkloads() = %+v, want accepts_app_workloads=true accepts_build_workloads=false", got)
	}
}

func TestClient_CreateNodeJoinToken(t *testing.T) {
	var gotMethod, gotPath string
	expires := time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(CreateNodeJoinTokenResponse{Token: "njtok_secret", ExpiresAt: expires})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.CreateNodeJoinToken(context.Background())
	if err != nil {
		t.Fatalf("CreateNodeJoinToken() error = %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/nodes/join-tokens" {
		t.Errorf("request = %s %s, want POST /api/v1/nodes/join-tokens", gotMethod, gotPath)
	}
	if got.Token != "njtok_secret" {
		t.Errorf("Token = %q, want njtok_secret", got.Token)
	}
	if !got.ExpiresAt.Equal(expires) {
		t.Errorf("ExpiresAt = %v, want %v", got.ExpiresAt, expires)
	}
}

func TestClient_CordonNode(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(NodeResource{ID: "nd_1", Schedulable: false})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.CordonNode(context.Background(), "nd_1")
	if err != nil {
		t.Fatalf("CordonNode() error = %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/nodes/nd_1/cordon" {
		t.Errorf("request = %s %s, want POST /api/v1/nodes/nd_1/cordon", gotMethod, gotPath)
	}
	if got.Schedulable {
		t.Errorf("Schedulable = true, want false after cordon")
	}
}

func TestClient_UncordonNode(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(NodeResource{ID: "nd_1", Schedulable: true})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.UncordonNode(context.Background(), "nd_1")
	if err != nil {
		t.Fatalf("UncordonNode() error = %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/nodes/nd_1/uncordon" {
		t.Errorf("request = %s %s, want POST /api/v1/nodes/nd_1/uncordon", gotMethod, gotPath)
	}
	if !got.Schedulable {
		t.Errorf("Schedulable = false, want true after uncordon")
	}
}

func TestClient_DrainNode_WithTarget(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DrainNodeResponse{TargetNodeID: "nd_2", MovedServices: []string{"web"}})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.DrainNode(context.Background(), "nd_1", "nd_2")
	if err != nil {
		t.Fatalf("DrainNode() error = %v", err)
	}
	if gotPath != "/api/v1/nodes/nd_1/drain" {
		t.Errorf("path = %q, want /api/v1/nodes/nd_1/drain", gotPath)
	}
	if gotQuery != "target_node_id=nd_2" {
		t.Errorf("query = %q, want target_node_id=nd_2", gotQuery)
	}
	if len(got.MovedServices) != 1 || got.MovedServices[0] != "web" {
		t.Errorf("MovedServices = %v, want [web]", got.MovedServices)
	}
}

func TestClient_DrainNode_DefaultTargetOmitsQuery(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DrainNodeResponse{})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	if _, err := client.DrainNode(context.Background(), "nd_1", ""); err != nil {
		t.Fatalf("DrainNode() error = %v", err)
	}
	if gotQuery != "" {
		t.Errorf("query = %q, want empty when targetNodeID is empty", gotQuery)
	}
}

func TestClient_DrainNode_PartialFailureIsNotAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMultiStatus)
		_ = json.NewEncoder(w).Encode(DrainNodeResponse{
			MovedServices: []string{"web"},
			Errors:        []string{"database main: connection refused"},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.DrainNode(context.Background(), "nd_1", "")
	if err != nil {
		t.Fatalf("DrainNode() error = %v, want no error for a 207 partial-failure response", err)
	}
	if len(got.Errors) != 1 {
		t.Errorf("Errors = %v, want one per-resource error", got.Errors)
	}
}

func TestClient_GetNodeHealth(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]ConditionResource{{Type: "Heartbeat", Status: "True"}})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.GetNodeHealth(context.Background(), "nd_1")
	if err != nil {
		t.Fatalf("GetNodeHealth() error = %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/api/v1/nodes/nd_1/health" {
		t.Errorf("request = %s %s, want GET /api/v1/nodes/nd_1/health", gotMethod, gotPath)
	}
	if len(got) != 1 || got[0].Type != "Heartbeat" {
		t.Errorf("GetNodeHealth() = %+v, want one Heartbeat condition", got)
	}
}
