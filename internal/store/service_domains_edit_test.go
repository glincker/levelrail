package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestEditServiceDomains(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "x:1", Port: 80, Env: map[string]string{"A": "1"}, Domains: []string{"a.com"}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "other", Image: "x:1", Port: 80, Domains: []string{"taken.com"}}); err != nil {
		t.Fatal(err)
	}
	add := func(d string) func([]string) ([]string, error) {
		return func(cur []string) ([]string, error) { return append(cur, d), nil }
	}

	got, changed, err := db.EditServiceDomains(ctx, "web", add("b.com"))
	if err != nil || !changed || strings.Join(got, ",") != "a.com,b.com" {
		t.Fatalf("add = %v %v %v", got, changed, err)
	}
	svc, _ := db.GetDesiredService(ctx, "web")
	if svc.Image != "x:1" || svc.Env["A"] != "1" || strings.Join(svc.Domains, ",") != "a.com,b.com" {
		t.Fatalf("stored = %+v", svc)
	}

	if _, changed, err := db.EditServiceDomains(ctx, "web", func(cur []string) ([]string, error) { return cur, nil }); err != nil || changed {
		t.Fatalf("no-op = %v %v", changed, err)
	}
	var taken *ErrDomainTaken
	if _, _, err := db.EditServiceDomains(ctx, "web", add("taken.com")); !errors.As(err, &taken) {
		t.Fatalf("taken = %v", err)
	}
	if _, _, err := db.EditServiceDomains(ctx, "missing", add("c.com")); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("missing = %v", err)
	}
	boom := errors.New("boom")
	if _, _, err := db.EditServiceDomains(ctx, "web", func([]string) ([]string, error) { return nil, boom }); !errors.Is(err, boom) {
		t.Fatalf("edit error = %v", err)
	}
}
