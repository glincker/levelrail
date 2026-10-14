package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newTestSAMLConnection(t *testing.T, db *DB) SAMLConnection {
	t.Helper()
	c := SAMLConnection{ID: "saml_1", Name: "Okta", AllowedDomains: []string{"example.com"}, DefaultRoleID: "role_viewer"}
	if err := db.CreateSAMLConnection(context.Background(), c, time.Now()); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSAMLConnectionCRUD(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	c := newTestSAMLConnection(t, db)
	if err := db.CreateSAMLConnection(ctx, SAMLConnection{ID: "saml_2", Name: "Okta"}, time.Now()); !errors.Is(err, ErrSAMLConnectionNameTaken) {
		t.Fatalf("duplicate name: %v", err)
	}
	c.Enabled, c.JITProvisioning, c.IDPEntityID = true, true, "https://idp"
	if err := db.UpdateSAMLConnection(ctx, c, time.Now()); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetSAMLConnection(ctx, c.ID)
	if err != nil || !got.Enabled || !got.JITProvisioning || got.IDPEntityID != "https://idp" || got.AllowedDomains[0] != "example.com" {
		t.Fatalf("got %+v err %v", got, err)
	}
	if err := db.DeleteSAMLConnection(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetSAMLConnection(ctx, c.ID); !errors.Is(err, ErrSAMLConnectionNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}

func TestSAMLAuthnRequestIsSingleUseAndBrowserBound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	c := newTestSAMLConnection(t, db)
	now := time.Now()
	if err := db.SaveSAMLAuthnRequest(ctx, "id-1", c.ID, "browser-a", now.Add(time.Minute), now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ConsumeSAMLAuthnRequest(ctx, c.ID, "browser-b", now); !errors.Is(err, ErrSAMLRequestNotFound) {
		t.Fatalf("other browser consumed the request: %v", err)
	}
	id, err := db.ConsumeSAMLAuthnRequest(ctx, c.ID, "browser-a", now)
	if err != nil || id != "id-1" {
		t.Fatalf("consume = %q, %v", id, err)
	}
	if _, err := db.ConsumeSAMLAuthnRequest(ctx, c.ID, "browser-a", now); !errors.Is(err, ErrSAMLRequestNotFound) {
		t.Fatalf("second consume: %v", err)
	}
	if err := db.SaveSAMLAuthnRequest(ctx, "id-2", c.ID, "browser-a", now.Add(time.Minute), now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ConsumeSAMLAuthnRequest(ctx, c.ID, "browser-a", now.Add(2*time.Minute)); !errors.Is(err, ErrSAMLRequestNotFound) {
		t.Fatalf("expired request consumed: %v", err)
	}
}

func TestRecordSAMLAssertionRefusesReplay(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now()
	if err := db.RecordSAMLAssertion(ctx, "saml_1", "a-1", now.Add(time.Minute), now); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordSAMLAssertion(ctx, "saml_1", "a-1", now.Add(time.Minute), now); !errors.Is(err, ErrSAMLAssertionReplayed) {
		t.Fatalf("replay: %v", err)
	}
	if err := db.RecordSAMLAssertion(ctx, "saml_2", "a-1", now.Add(time.Minute), now); err != nil {
		t.Fatalf("same id on another connection: %v", err)
	}
}

func TestCreateProvisionedUserIsAtomic(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	c := newTestSAMLConnection(t, db)
	u := User{ID: "user_p1", Email: "p1@example.com", DisplayName: "P1", CreatedAt: time.Now()}
	if err := db.CreateProvisionedUser(ctx, ProvisionedUser{User: u, RoleID: "role_operator", SAMLConnectionID: c.ID, SAMLNameID: "nid-1"}); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetUserByID(ctx, u.ID)
	if err != nil || len(got.Abilities) != 4 {
		t.Fatalf("user %+v err %v", got, err)
	}
	if role, _ := db.UserRoleID(ctx, u.ID); role != "role_operator" {
		t.Fatalf("role = %q", role)
	}
	if uid, err := db.SAMLIdentityUser(ctx, c.ID, "nid-1"); err != nil || uid != u.ID {
		t.Fatalf("identity = %q %v", uid, err)
	}
	dup := ProvisionedUser{User: User{ID: "user_p2", Email: "p2@example.com", CreatedAt: time.Now()}, RoleID: "role_viewer", SAMLConnectionID: c.ID, SAMLNameID: "nid-1"}
	if err := db.CreateProvisionedUser(ctx, dup); err == nil {
		t.Fatal("linked one subject twice")
	}
	if _, err := db.GetUserByID(ctx, "user_p2"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("failed provisioning left a user behind: %v", err)
	}
	if err := db.CreateProvisionedUser(ctx, ProvisionedUser{User: User{ID: "user_p3", Email: "p3@example.com", CreatedAt: time.Now()}, RoleID: "role_missing"}); !errors.Is(err, ErrRoleNotFound) {
		t.Fatalf("unknown role: %v", err)
	}
}

func TestReplaceSSOGroupMappings(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	set := []SSOGroupMapping{{Group: "Admins", RoleID: "role_admin", Priority: 10}, {Group: "eng", RoleID: "role_operator"}}
	if err := db.ReplaceSSOGroupMappings(ctx, set, time.Now()); err != nil {
		t.Fatal(err)
	}
	got, err := db.ListSSOGroupMappings(ctx)
	if err != nil || len(got) != 2 || got[0].Group != "Admins" {
		t.Fatalf("got %+v err %v", got, err)
	}
	if err := db.ReplaceSSOGroupMappings(ctx, []SSOGroupMapping{{Group: "a", RoleID: "role_viewer"}, {Group: "A", RoleID: "role_admin"}}, time.Now()); !errors.Is(err, ErrSSOGroupDuplicate) {
		t.Fatalf("duplicate: %v", err)
	}
	if err := db.ReplaceSSOGroupMappings(ctx, []SSOGroupMapping{{Group: "x", RoleID: "role_nope"}}, time.Now()); !errors.Is(err, ErrRoleNotFound) {
		t.Fatalf("unknown role: %v", err)
	}
	if got, _ := db.ListSSOGroupMappings(ctx); len(got) != 2 {
		t.Fatalf("failed replace changed the set: %+v", got)
	}
}

func TestSSOSettingsCertificateIsSetOnce(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	won, err := db.SetSSOSPCertificate(ctx, "cert-a", time.Now())
	if err != nil || !won {
		t.Fatalf("first set: %v %v", won, err)
	}
	if won, _ := db.SetSSOSPCertificate(ctx, "cert-b", time.Now()); won {
		t.Fatal("second set won")
	}
	s, err := db.GetSSOSettings(ctx)
	if err != nil || s.SPCertificatePEM != "cert-a" || s.EnforceSSO {
		t.Fatalf("settings %+v err %v", s, err)
	}
}
