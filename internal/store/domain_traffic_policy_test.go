package store

import (
	"context"
	"testing"
)

func TestDomainTrafficPolicyRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80, Domains: []string{"app.example.com", "www.example.com"},
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	if _, found, err := db.GetDomainTrafficPolicy(ctx, "app.example.com", "headers"); err != nil || found {
		t.Fatalf("GetDomainTrafficPolicy() found = %v, err = %v, want default", found, err)
	}
	for i, spec := range []string{`{"rules":[]}`, `{"rules":[{"side":"response","op":"set","name":"X-A","value":"1"}]}`} {
		if err := db.SetDomainTrafficPolicy(ctx, "app.example.com", "headers", []byte(spec)); err != nil {
			t.Fatalf("SetDomainTrafficPolicy() pass %d error = %v", i, err)
		}
	}
	got, found, err := db.GetDomainTrafficPolicy(ctx, "app.example.com", "headers")
	if err != nil || !found || string(got.Spec) != `{"rules":[{"side":"response","op":"set","name":"X-A","value":"1"}]}` || got.UpdatedAt == "" {
		t.Fatalf("GetDomainTrafficPolicy() = %+v, %v, %v", got, found, err)
	}
	if err := db.SetDomainTrafficPolicy(ctx, "app.example.com", "nope", []byte(`{}`)); err == nil {
		t.Error("SetDomainTrafficPolicy(unknown kind) succeeded, want CHECK failure")
	}
	if err := db.SetDomainTrafficPolicy(ctx, "unknown.example.com", "geo", []byte(`{}`)); err == nil {
		t.Error("SetDomainTrafficPolicy(unowned domain) succeeded, want foreign key failure")
	}
	if err := db.SetDomainTrafficPolicy(ctx, "www.example.com", "cache", []byte(`{"enabled":false}`)); err != nil {
		t.Fatalf("SetDomainTrafficPolicy(www) error = %v", err)
	}
	all, err := db.ListDomainTrafficPolicies(ctx)
	if err != nil || len(all) != 2 {
		t.Fatalf("ListDomainTrafficPolicies() = %v, %v, want 2 rows", all, err)
	}
	mine, err := db.ListDomainTrafficPoliciesFor(ctx, "app.example.com")
	if err != nil || len(mine) != 1 || mine[0].Kind != "headers" {
		t.Fatalf("ListDomainTrafficPoliciesFor() = %v, %v", mine, err)
	}
	for i := 0; i < 2; i++ {
		if err := db.DeleteDomainTrafficPolicy(ctx, "app.example.com", "headers"); err != nil {
			t.Fatalf("DeleteDomainTrafficPolicy() pass %d error = %v", i, err)
		}
	}
	if _, found, _ := db.GetDomainTrafficPolicy(ctx, "app.example.com", "headers"); found {
		t.Error("policy still present after delete")
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v2", Port: 80}); err != nil {
		t.Fatalf("SaveDesiredService() (drop domains) error = %v", err)
	}
	if all, _ := db.ListDomainTrafficPolicies(ctx); len(all) != 0 {
		t.Errorf("rows survived domain removal: %v", all)
	}
}
