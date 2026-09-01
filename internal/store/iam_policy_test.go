package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func testPolicy(id, name string) Policy {
	return Policy{
		ID:          id,
		Name:        name,
		Description: "test policy",
		Document:    `{"Statement":[{"Effect":"Allow","Action":["read"],"Resource":["app:web"]}]}`,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

func TestSaveAndGetPolicy(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := testPolicy("pol_1", "web-readers")
	if err := db.SavePolicy(ctx, want); err != nil {
		t.Fatalf("SavePolicy() error = %v", err)
	}

	got, err := db.GetPolicy(ctx, "pol_1")
	if err != nil {
		t.Fatalf("GetPolicy() error = %v", err)
	}
	if got.Name != want.Name || got.Description != want.Description || got.Document != want.Document {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestGetPolicy_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if _, err := db.GetPolicy(ctx, "missing"); !errors.Is(err, ErrPolicyNotFound) {
		t.Errorf("GetPolicy() error = %v, want ErrPolicyNotFound", err)
	}
}

func TestSavePolicy_DuplicateName(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SavePolicy(ctx, testPolicy("pol_1", "dup-name")); err != nil {
		t.Fatalf("SavePolicy() error = %v", err)
	}
	err := db.SavePolicy(ctx, testPolicy("pol_2", "dup-name"))
	if !errors.Is(err, ErrPolicyNameExists) {
		t.Errorf("SavePolicy() error = %v, want ErrPolicyNameExists", err)
	}
}

func TestUpdatePolicy(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SavePolicy(ctx, testPolicy("pol_1", "original")); err != nil {
		t.Fatalf("SavePolicy() error = %v", err)
	}

	newDoc := `{"Statement":[{"Effect":"Deny","Action":["write"],"Resource":["*"]}]}`
	if err := db.UpdatePolicy(ctx, "pol_1", "renamed", "updated desc", newDoc); err != nil {
		t.Fatalf("UpdatePolicy() error = %v", err)
	}

	got, err := db.GetPolicy(ctx, "pol_1")
	if err != nil {
		t.Fatalf("GetPolicy() error = %v", err)
	}
	if got.Name != "renamed" || got.Description != "updated desc" || got.Document != newDoc {
		t.Errorf("got %+v after update", got)
	}
	if !got.UpdatedAt.After(got.CreatedAt.Add(-time.Second)) {
		t.Errorf("UpdatedAt = %v, want it set", got.UpdatedAt)
	}
}

func TestUpdatePolicy_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.UpdatePolicy(ctx, "missing", "x", "y", `{"Statement":[]}`)
	if !errors.Is(err, ErrPolicyNotFound) {
		t.Errorf("UpdatePolicy() error = %v, want ErrPolicyNotFound", err)
	}
}

func TestUpdatePolicy_DuplicateName(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SavePolicy(ctx, testPolicy("pol_1", "first")); err != nil {
		t.Fatalf("SavePolicy() error = %v", err)
	}
	if err := db.SavePolicy(ctx, testPolicy("pol_2", "second")); err != nil {
		t.Fatalf("SavePolicy() error = %v", err)
	}

	err := db.UpdatePolicy(ctx, "pol_2", "first", "desc", `{"Statement":[]}`)
	if !errors.Is(err, ErrPolicyNameExists) {
		t.Errorf("UpdatePolicy() error = %v, want ErrPolicyNameExists", err)
	}
}

func TestListPolicies_OrderedByName(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SavePolicy(ctx, testPolicy("pol_1", "zeta")); err != nil {
		t.Fatalf("SavePolicy() error = %v", err)
	}
	if err := db.SavePolicy(ctx, testPolicy("pol_2", "alpha")); err != nil {
		t.Fatalf("SavePolicy() error = %v", err)
	}

	got, err := db.ListPolicies(ctx)
	if err != nil {
		t.Fatalf("ListPolicies() error = %v", err)
	}
	if len(got) != 2 || got[0].Name != "alpha" || got[1].Name != "zeta" {
		t.Errorf("got %+v, want [alpha zeta]", got)
	}
}

func TestDeletePolicy_CascadesAttachments(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SavePolicy(ctx, testPolicy("pol_1", "to-delete")); err != nil {
		t.Fatalf("SavePolicy() error = %v", err)
	}
	if err := db.AttachPolicy(ctx, "att_1", "pol_1", PrincipalTypeUser, "user_1"); err != nil {
		t.Fatalf("AttachPolicy() error = %v", err)
	}

	if err := db.DeletePolicy(ctx, "pol_1"); err != nil {
		t.Fatalf("DeletePolicy() error = %v", err)
	}

	if _, err := db.GetPolicy(ctx, "pol_1"); !errors.Is(err, ErrPolicyNotFound) {
		t.Errorf("GetPolicy() after delete error = %v, want ErrPolicyNotFound", err)
	}
	attachments, err := db.ListAttachmentsForPolicy(ctx, "pol_1")
	if err != nil {
		t.Fatalf("ListAttachmentsForPolicy() error = %v", err)
	}
	if len(attachments) != 0 {
		t.Errorf("ListAttachmentsForPolicy() after cascade delete = %+v, want empty", attachments)
	}
}

func TestDeletePolicy_Idempotent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.DeletePolicy(ctx, "missing"); err != nil {
		t.Errorf("DeletePolicy() on missing id error = %v, want nil", err)
	}
}

func TestAttachPolicy_Idempotent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SavePolicy(ctx, testPolicy("pol_1", "attach-idempotent")); err != nil {
		t.Fatalf("SavePolicy() error = %v", err)
	}
	if err := db.AttachPolicy(ctx, "att_1", "pol_1", PrincipalTypeUser, "user_1"); err != nil {
		t.Fatalf("AttachPolicy() error = %v", err)
	}
	if err := db.AttachPolicy(ctx, "att_2", "pol_1", PrincipalTypeUser, "user_1"); err != nil {
		t.Fatalf("AttachPolicy() second call error = %v, want nil (idempotent)", err)
	}

	attachments, err := db.ListAttachmentsForPolicy(ctx, "pol_1")
	if err != nil {
		t.Fatalf("ListAttachmentsForPolicy() error = %v", err)
	}
	if len(attachments) != 1 {
		t.Errorf("ListAttachmentsForPolicy() = %+v, want exactly one attachment", attachments)
	}
}

func TestDetachPolicy_Idempotent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SavePolicy(ctx, testPolicy("pol_1", "detach-idempotent")); err != nil {
		t.Fatalf("SavePolicy() error = %v", err)
	}
	if err := db.DetachPolicy(ctx, "pol_1", PrincipalTypeUser, "user_1"); err != nil {
		t.Errorf("DetachPolicy() on non-attached principal error = %v, want nil", err)
	}

	if err := db.AttachPolicy(ctx, "att_1", "pol_1", PrincipalTypeUser, "user_1"); err != nil {
		t.Fatalf("AttachPolicy() error = %v", err)
	}
	if err := db.DetachPolicy(ctx, "pol_1", PrincipalTypeUser, "user_1"); err != nil {
		t.Fatalf("DetachPolicy() error = %v", err)
	}
	attachments, err := db.ListAttachmentsForPolicy(ctx, "pol_1")
	if err != nil {
		t.Fatalf("ListAttachmentsForPolicy() error = %v", err)
	}
	if len(attachments) != 0 {
		t.Errorf("ListAttachmentsForPolicy() after detach = %+v, want empty", attachments)
	}
}

func TestListPoliciesForPrincipal(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SavePolicy(ctx, testPolicy("pol_1", "for-user")); err != nil {
		t.Fatalf("SavePolicy() error = %v", err)
	}
	if err := db.SavePolicy(ctx, testPolicy("pol_2", "for-token")); err != nil {
		t.Fatalf("SavePolicy() error = %v", err)
	}
	if err := db.AttachPolicy(ctx, "att_1", "pol_1", PrincipalTypeUser, "user_1"); err != nil {
		t.Fatalf("AttachPolicy() error = %v", err)
	}
	if err := db.AttachPolicy(ctx, "att_2", "pol_2", PrincipalTypeToken, "tok_1"); err != nil {
		t.Fatalf("AttachPolicy() error = %v", err)
	}

	userPolicies, err := db.ListPoliciesForPrincipal(ctx, PrincipalTypeUser, "user_1")
	if err != nil {
		t.Fatalf("ListPoliciesForPrincipal(user) error = %v", err)
	}
	if len(userPolicies) != 1 || userPolicies[0].Name != "for-user" {
		t.Errorf("ListPoliciesForPrincipal(user) = %+v, want [for-user]", userPolicies)
	}

	tokenPolicies, err := db.ListPoliciesForPrincipal(ctx, PrincipalTypeToken, "tok_1")
	if err != nil {
		t.Fatalf("ListPoliciesForPrincipal(token) error = %v", err)
	}
	if len(tokenPolicies) != 1 || tokenPolicies[0].Name != "for-token" {
		t.Errorf("ListPoliciesForPrincipal(token) = %+v, want [for-token]", tokenPolicies)
	}

	none, err := db.ListPoliciesForPrincipal(ctx, PrincipalTypeUser, "user_2")
	if err != nil {
		t.Fatalf("ListPoliciesForPrincipal(no attachments) error = %v", err)
	}
	if len(none) != 0 {
		t.Errorf("ListPoliciesForPrincipal(no attachments) = %+v, want empty", none)
	}
}

func TestNewPolicyID_Unique(t *testing.T) {
	a, err := NewPolicyID()
	if err != nil {
		t.Fatalf("NewPolicyID() error = %v", err)
	}
	b, err := NewPolicyID()
	if err != nil {
		t.Fatalf("NewPolicyID() error = %v", err)
	}
	if a == b {
		t.Errorf("NewPolicyID() returned duplicate values: %v", a)
	}
	if len(a) <= len("pol_") {
		t.Errorf("NewPolicyID() = %q, want it to carry the pol_ prefix plus a real suffix", a)
	}
}
