package store

import (
	"context"
	"testing"
	"time"
)

// TestSaveDesiredService_KeptDomainKeepsItsSettings covers a re-save that
// keeps a domain (an env change, a deploy, a rollback): every per-domain
// setting cascades from service_domains, so none may be dropped.
func TestSaveDesiredService_KeptDomainKeepsItsSettings(t *testing.T) {
	const domain = "www.example.com"
	tests := []struct {
		name  string
		set   func(ctx context.Context, db *DB) error
		found func(ctx context.Context, db *DB) (bool, error)
	}{
		{
			name: "redirect",
			set: func(ctx context.Context, db *DB) error {
				return db.SetDomainRedirect(ctx, domain, "https://example.com", DomainRedirectPermanent)
			},
			found: func(ctx context.Context, db *DB) (bool, error) {
				_, ok, err := db.GetDomainRedirect(ctx, domain)
				return ok, err
			},
		},
		{
			name: "basic auth",
			set:  func(ctx context.Context, db *DB) error { return db.SetDomainBasicAuth(ctx, domain, "admin") },
			found: func(ctx context.Context, db *DB) (bool, error) {
				_, ok, err := db.GetDomainBasicAuth(ctx, domain)
				return ok, err
			},
		},
		{
			name:  "maintenance",
			set:   func(ctx context.Context, db *DB) error { return db.SetDomainMaintenance(ctx, domain) },
			found: func(ctx context.Context, db *DB) (bool, error) { return db.GetDomainMaintenance(ctx, domain) },
		},
		{
			name: "tls cert",
			set: func(ctx context.Context, db *DB) error {
				now := time.Now().UTC()
				return db.SetDomainTLSCert(ctx, domain, now, now.Add(90*24*time.Hour))
			},
			found: func(ctx context.Context, db *DB) (bool, error) {
				_, ok, err := db.GetDomainTLSCert(ctx, domain)
				return ok, err
			},
		},
		{
			name: "waf",
			set: func(ctx context.Context, db *DB) error {
				return db.SetDomainWAF(ctx, domain, true, DomainWAFModeBlock, 10, 20)
			},
			found: func(ctx context.Context, db *DB) (bool, error) {
				_, ok, err := db.GetDomainWAF(ctx, domain)
				return ok, err
			},
		},
		{
			name: "error page",
			set: func(ctx context.Context, db *DB) error {
				return db.SetDomainErrorPage(ctx, domain, 404, "<h1>gone</h1>")
			},
			found: func(ctx context.Context, db *DB) (bool, error) {
				pages, err := db.ListDomainErrorPages(ctx, domain)
				return len(pages) == 1, err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			ctx := context.Background()
			svc := DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{domain, "example.com"}}
			if err := db.SaveDesiredService(ctx, svc); err != nil {
				t.Fatalf("SaveDesiredService() error = %v", err)
			}
			if err := tt.set(ctx, db); err != nil {
				t.Fatalf("set %s: %v", tt.name, err)
			}

			svc.Image, svc.Env = "img:v2", map[string]string{"NEW": "1"}
			svc.Domains = []string{"example.com", domain, "api.example.com"}
			if err := db.SaveDesiredService(ctx, svc); err != nil {
				t.Fatalf("SaveDesiredService() (re-save) error = %v", err)
			}

			ok, err := tt.found(ctx, db)
			if err != nil {
				t.Fatalf("read %s: %v", tt.name, err)
			}
			if !ok {
				t.Errorf("%s setting on %s was dropped by a re-save that kept the domain", tt.name, domain)
			}
		})
	}
}

func TestSaveDesiredService_DomainClaims(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	save := func(name string, domains ...string) error {
		return db.SaveDesiredService(ctx, DesiredService{Name: name, Image: "img:v1", Port: 80, Domains: domains})
	}
	if err := save("web", "a.example.com", "b.example.com", "a.example.com"); err != nil {
		t.Fatalf("save web: %v", err)
	}
	if err := save("web", "b.example.com"); err != nil {
		t.Fatalf("re-save web: %v", err)
	}
	if err := save("api", "a.example.com"); err != nil {
		t.Errorf("claim released domain: %v, want it free once web dropped it", err)
	}
	if err := save("other", "b.example.com"); err == nil {
		t.Error("claim a domain web still holds: error = nil, want ErrDomainTaken")
	}
}
