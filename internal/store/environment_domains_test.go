package store

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestDomainRewrite(t *testing.T) {
	tests := []struct {
		name string
		rw   DomainRewrite
		in   []string
		want []string
	}{
		{"prefix", DomainRewrite{Prefix: "uat."}, []string{"example.com", "www.example.com"}, []string{"uat.example.com", "uat.www.example.com"}},
		{"suffix swap", DomainRewrite{Find: "example.com", Replace: "uat.example.com"}, []string{"example.com", "api.example.com", "other.org"}, []string{"uat.example.com", "api.uat.example.com", "other.org"}},
		{"no match unchanged", DomainRewrite{Find: "example.com", Replace: "x.io"}, []string{"notexample.com"}, []string{"notexample.com"}},
		{"empty keeps", DomainRewrite{}, []string{"A.com", "a.com"}, []string{"a.com"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rw.ApplyAll(tt.in); !slices.Equal(got, tt.want) {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}

func TestEffectiveServiceDomains(t *testing.T) {
	sets := map[string][]string{"env_dev": {"dev.a.com"}, "env_empty": {}}
	tests := []struct {
		name, env string
		want      []string
	}{
		{"own set wins", "env_dev", []string{"dev.a.com"}},
		{"unknown env falls back", "env_prod", []string{"a.com"}},
		{"empty set falls back", "env_empty", []string{"a.com"}},
		{"untagged falls back", "", []string{"a.com"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EffectiveServiceDomains([]string{"a.com"}, tt.env, sets); !slices.Equal(got, tt.want) {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}

func TestEditServiceEnvironmentDomains(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "x:1", Port: 80, Domains: []string{"a.com"}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "other", Image: "x:1", Port: 80, Domains: []string{"taken.com"}}); err != nil {
		t.Fatal(err)
	}
	set := func(d ...string) func([]string) ([]string, error) {
		return func([]string) ([]string, error) { return d, nil }
	}

	got, changed, err := db.EditServiceEnvironmentDomains(ctx, "web", "env_dev", set("dev.a.com"))
	if err != nil || !changed || !slices.Equal(got, []string{"dev.a.com"}) {
		t.Fatalf("set = %v %v %v", got, changed, err)
	}
	if _, changed, err := db.EditServiceEnvironmentDomains(ctx, "web", "env_dev", set("dev.a.com")); err != nil || changed {
		t.Fatalf("no-op = %v %v", changed, err)
	}

	var taken *ErrDomainTaken
	if _, _, err := db.EditServiceEnvironmentDomains(ctx, "web", "env_uat", set("taken.com")); !errors.As(err, &taken) {
		t.Fatalf("taken = %v", err)
	}
	if _, _, err := db.EditServiceEnvironmentDomains(ctx, "other", "env_uat", set("dev.a.com")); !errors.As(err, &taken) {
		t.Fatalf("cross-service claim = %v", err)
	}
	if _, _, err := db.EditServiceEnvironmentDomains(ctx, "missing", "env_dev", set("x.com")); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("missing app = %v", err)
	}
	if _, _, err := db.EditServiceEnvironmentDomains(ctx, "web", "env_nope", set("x.com")); !errors.Is(err, ErrEnvironmentNotFound) {
		t.Fatalf("missing env = %v", err)
	}

	// A default-set save must not release the environment set's claim.
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "x:2", Port: 80, Domains: []string{"a.com"}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "other", Image: "x:1", Port: 80, Domains: []string{"dev.a.com"}}); !errors.As(err, &taken) {
		t.Fatalf("claim released by default-set save: %v", err)
	}

	all, err := db.ListAllServiceEnvironmentDomains(ctx)
	if err != nil || !slices.Equal(all["web"]["env_dev"], []string{"dev.a.com"}) {
		t.Fatalf("list all = %v %v", all, err)
	}

	// Clearing the set frees the hostname.
	if _, _, err := db.EditServiceEnvironmentDomains(ctx, "web", "env_dev", set()); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "other", Image: "x:1", Port: 80, Domains: []string{"dev.a.com"}}); err != nil {
		t.Fatalf("hostname still claimed after clear: %v", err)
	}
}

func TestDeleteEnvironmentReleasesDomainClaims(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "x:1", Port: 80, Domains: []string{"a.com"}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveEnvironment(ctx, Environment{ID: "env_tmp", ProjectID: "proj_global", Name: "tmp"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.EditServiceEnvironmentDomains(ctx, "web", "env_tmp", func([]string) ([]string, error) { return []string{"tmp.a.com"}, nil }); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteEnvironment(ctx, "env_tmp"); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "other", Image: "x:1", Port: 80, Domains: []string{"tmp.a.com"}}); err != nil {
		t.Fatalf("claim survived environment delete: %v", err)
	}
}
