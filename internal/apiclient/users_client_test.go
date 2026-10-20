package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_CreateUser_WithRole(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody CreateUserRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(UserResource{ID: "user_1", Email: gotBody.Email, Role: gotBody.Role})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.CreateUser(context.Background(), CreateUserRequest{
		Email: "teammate@example.com", Password: "a-real-password", Role: "operator",
	})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/auth/users" {
		t.Errorf("request = %s %s, want POST /api/v1/auth/users", gotMethod, gotPath)
	}
	if gotBody.Role != "operator" {
		t.Errorf("request body Role = %q, want %q", gotBody.Role, "operator")
	}
	if got.ID != "user_1" || got.Role != "operator" {
		t.Errorf("CreateUser() = %+v, want id user_1, role operator", got)
	}
}

func TestClient_ListUsers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]UserResource{{ID: "user_1", Email: "a@example.com"}})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "user_1" {
		t.Errorf("ListUsers() = %+v, want one user user_1", got)
	}
}

func TestClient_UpdateUserAbilities_WithRole(t *testing.T) {
	var gotPath string
	var gotBody UpdateUserAbilitiesRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(UserResource{ID: "user_1", Role: gotBody.Role})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.UpdateUserAbilities(context.Background(), "user_1", UpdateUserAbilitiesRequest{Role: "viewer"})
	if err != nil {
		t.Fatalf("UpdateUserAbilities() error = %v", err)
	}
	if gotPath != "/api/v1/users/user_1/abilities" {
		t.Errorf("path = %s, want /api/v1/users/user_1/abilities", gotPath)
	}
	if got.Role != "viewer" {
		t.Errorf("UpdateUserAbilities() Role = %q, want %q", got.Role, "viewer")
	}
}

func TestClient_DeleteUser_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"user not found"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	err := client.DeleteUser(context.Background(), "user_missing")
	if err == nil {
		t.Fatal("DeleteUser() error = nil, want an error for a 404 response")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("error = %v, want *APIError with status 404", err)
	}
}

func TestClient_ListRoles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/roles" {
			t.Errorf("path = %s, want /api/v1/roles", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]RoleResource{
			{Name: "admin", Description: "full access", Abilities: []string{"root"}},
			{Name: "operator", Description: "deploy and manage", Abilities: []string{"read", "read:sensitive", "write", "deploy"}},
			{Name: "viewer", Description: "read only", Abilities: []string{"read"}},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.ListRoles(context.Background())
	if err != nil {
		t.Fatalf("ListRoles() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("ListRoles() = %d roles, want 3", len(got))
	}
}
