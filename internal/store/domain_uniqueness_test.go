package store

import (
	"context"
	"errors"
	"testing"
)

func TestSaveDesiredService_DomainTaken_RejectsWrite(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"},
	}); err != nil {
		t.Fatalf("SaveDesiredService(web) error = %v", err)
	}

	err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web2", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"},
	})
	var taken *ErrDomainTaken
	if !errors.As(err, &taken) {
		t.Fatalf("SaveDesiredService(web2) error = %v, want *ErrDomainTaken", err)
	}
	if taken.Domain != "app.example.com" || taken.Owner != "web" {
		t.Errorf("ErrDomainTaken = %+v, want Domain=app.example.com Owner=web", taken)
	}

	// The rejected write must be a no-op: web2 was never created at all,
	// this is the "operation half-succeeded" case CLAUDE.md 7 requires a
	// test for, applied to a store write instead of a reconciler.
	if _, err := db.GetDesiredService(ctx, "web2"); !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("GetDesiredService(web2) error = %v, want ErrServiceNotFound: a rejected domain claim must not partially create the service", err)
	}
}

func TestSaveDesiredService_DomainTaken_PartialClaimDoesNotLeakFreeDomains(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80, Domains: []string{"taken.example.com"},
	}); err != nil {
		t.Fatalf("SaveDesiredService(web) error = %v", err)
	}

	// web2 claims one free domain and one already-taken domain in the
	// same write. The whole write must roll back, so "free.example.com"
	// must not end up claimed by web2 either, half-succeeded in the
	// other direction (claim the free one, silently drop the taken one)
	// would be just as wrong as reporting failure but keeping the claim.
	err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web2", Image: "img:v1", Port: 80,
		Domains: []string{"free.example.com", "taken.example.com"},
	})
	var taken *ErrDomainTaken
	if !errors.As(err, &taken) {
		t.Fatalf("SaveDesiredService(web2) error = %v, want *ErrDomainTaken", err)
	}

	if _, err := db.GetDesiredService(ctx, "web2"); !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("GetDesiredService(web2) error = %v, want ErrServiceNotFound", err)
	}

	// free.example.com must still be free: a third service claiming only
	// that domain must succeed.
	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web3", Image: "img:v1", Port: 80, Domains: []string{"free.example.com"},
	}); err != nil {
		t.Errorf("SaveDesiredService(web3) claiming free.example.com error = %v, want nil: the rolled-back web2 write must not have leaked a partial claim on it", err)
	}
}

func TestSaveDesiredService_Redeploy_KeepsOwnDomain(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	svc := DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"}}
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Fatalf("initial SaveDesiredService() error = %v", err)
	}

	svc.Image = "img:v2"
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Errorf("redeploy SaveDesiredService() with the same domain error = %v, want nil: a service re-claiming its own domain must not conflict with itself", err)
	}
}

func TestSaveDesiredService_DomainReleasedWhenRemoved(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80, Domains: []string{"a.example.com", "b.example.com"},
	}); err != nil {
		t.Fatalf("initial SaveDesiredService() error = %v", err)
	}

	// web drops b.example.com on redeploy.
	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v2", Port: 80, Domains: []string{"a.example.com"},
	}); err != nil {
		t.Fatalf("redeploy dropping a domain error = %v", err)
	}

	// b.example.com must now be free for a different service to claim.
	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web2", Image: "img:v1", Port: 80, Domains: []string{"b.example.com"},
	}); err != nil {
		t.Errorf("SaveDesiredService(web2) claiming a domain web released error = %v, want nil", err)
	}
}

func TestSaveDesiredService_DuplicateDomainWithinOwnList(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// Two identical entries in the same service's own Domains slice must
	// not be treated as a self-conflict.
	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Domains: []string{"app.example.com", "app.example.com"},
	}); err != nil {
		t.Errorf("SaveDesiredService() with a duplicate domain in its own list error = %v, want nil", err)
	}
}

func TestSaveDesiredService_NoDomains_NeverConflicts(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "worker1", Image: "img:v1", Port: 0}); err != nil {
		t.Fatalf("SaveDesiredService(worker1) error = %v", err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "worker2", Image: "img:v1", Port: 0}); err != nil {
		t.Errorf("SaveDesiredService(worker2) error = %v, want nil: two domain-less services must never conflict", err)
	}
}

func TestDeleteDesiredService_ReleasesItsDomains(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"},
	}); err != nil {
		t.Fatalf("SaveDesiredService(web) error = %v", err)
	}
	if err := db.DeleteDesiredService(ctx, "web"); err != nil {
		t.Fatalf("DeleteDesiredService(web) error = %v", err)
	}

	// service_domains has a foreign key to desired_services with ON
	// DELETE CASCADE (migrations/0011); deleting the owning service must
	// free its domain for a different service to claim.
	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web2", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"},
	}); err != nil {
		t.Errorf("SaveDesiredService(web2) after deleting the prior owner error = %v, want nil", err)
	}
}
