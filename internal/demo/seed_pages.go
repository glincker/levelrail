package demo

import (
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type pagesSiteSpec struct {
	name, host, commit string
	spa                bool
}

var pagesSites = []pagesSiteSpec{
	{"acme-landing", "landing", "feat: add pricing comparison table", false},
	{"acme-handbook", "handbook", "docs: add on-call rotation guide", true},
}

var pagesCommitTitles = []string{
	"feat: add pricing comparison table",
	"fix: broken anchor on the changelog page",
	"docs: refresh the getting started guide",
	"chore: compress hero images",
	"feat: add contact form",
	"fix: canonical url on blog posts",
}

// pages seeds the Pages sites, their deploy history, forms inbox, previews and routing rules.
// It uses its own rng so the rest of the estate stays byte for byte the same.
func (s *seeder) pages() error {
	r := newRNG(s.opts.Seed ^ 0x70a6e5)
	for _, site := range pagesSites {
		if err := s.pagesSite(site, r); err != nil {
			return err
		}
	}
	return nil
}

func (s *seeder) pagesSite(site pagesSiteSpec, r *rng) error {
	ss := store.StaticSite{
		Name: site.name, Domains: []string{site.host + "." + s.opts.DomainSuffix},
		RootDir: "/var/lib/levelrail/static/" + site.name + "/current", UpdatedAt: s.at(1, 2*time.Hour),
	}
	if err := s.db.SaveStaticSite(s.ctx, ss); err != nil {
		return fmt.Errorf("demo: save static site %q: %w", site.name, err)
	}
	spa, clean := site.spa, true
	settings := store.StaticSiteSettings{OutputDir: "dist", BuildCommand: "npm run build", SPAFallback: &spa, CleanURLs: &clean, NotFoundPage: "404.html"}
	if err := s.db.SaveStaticSiteSettings(s.ctx, site.name, settings); err != nil {
		return fmt.Errorf("demo: save static site settings %q: %w", site.name, err)
	}
	rules := store.StaticUIRules{
		Redirects: []store.StaticUIRedirect{
			{From: "/old-pricing", To: "/pricing", Status: 301},
			{From: "/blog/launch", To: "/blog/launch-notes", Status: 308},
		},
		Headers: []store.StaticUIHeader{{Path: "/assets/*", Set: map[string]string{"Cache-Control": "public, max-age=31536000, immutable"}}},
	}
	if err := s.db.SaveStaticSiteUIRules(s.ctx, site.name, rules); err != nil {
		return fmt.Errorf("demo: save static site rules %q: %w", site.name, err)
	}
	if err := s.pagesDeploys(site, r); err != nil {
		return err
	}
	if err := s.pagesPreviews(site, r); err != nil {
		return err
	}
	if site.name == "acme-landing" {
		return s.pagesForms(site, r)
	}
	return nil
}

func (s *seeder) pagesDeploys(site pagesSiteSpec, r *rng) error {
	authors := []string{userSpecs[0].name, userSpecs[1].name}
	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("dep_%s_%03d", site.name, i+1)
		at := s.at(12-i*2, time.Duration(9+r.intn(6))*time.Hour)
		status, failure := store.DeployAttemptStatusSucceeded, ""
		if i == 2 {
			status, failure = store.DeployAttemptStatusFailed, "build failed: npm run build exited 1 (missing file src/pages/pricing.md)"
		}
		sha := r.hex(7)
		da := store.DeployAttempt{
			ID: id, ServiceName: site.name, Image: "static:" + sha, CommitSHA: sha,
			Source: store.DeployAttemptSourceWebhook, Status: store.DeployAttemptStatusRunning, StartedAt: at,
			Sequence: int64(i + 1), Branch: "main", CommitMessage: pagesCommitTitles[(i+len(site.name))%len(pagesCommitTitles)],
			Author: authors[i%len(authors)],
		}
		if err := s.db.SaveDeployAttempt(s.ctx, da); err != nil {
			return fmt.Errorf("demo: save pages deploy %q: %w", id, err)
		}
		if err := s.db.FinishDeployAttempt(s.ctx, id, status, at.Add(time.Duration(25+r.intn(40))*time.Second), failure); err != nil {
			return fmt.Errorf("demo: finish pages deploy %q: %w", id, err)
		}
	}
	return nil
}

func (s *seeder) pagesPreviews(site pagesSiteSpec, r *rng) error {
	branches := []struct {
		n      int
		branch string
	}{{14, "feat/new-hero"}, {17, "docs/pricing-faq"}}
	for i, b := range branches {
		at := s.stamp(s.at(2-i, 5*time.Hour))
		p := store.PreviewEnvironment{
			ID: fmt.Sprintf("prev_%s_%d", site.name, b.n), AppName: site.name, PRNumber: b.n,
			PreviewAppID: fmt.Sprintf("%s-pr-%d", site.name, b.n), Branch: b.branch, HeadSHA: r.hex(7),
			Domain: fmt.Sprintf("pr-%d.%s.%s", b.n, site.host, s.opts.DomainSuffix),
			Status: store.PreviewStatusActive, CreatedAt: at, UpdatedAt: at,
		}
		if err := s.db.SavePreviewEnvironment(s.ctx, p); err != nil {
			return fmt.Errorf("demo: save pages preview: %w", err)
		}
	}
	return nil
}

var pagesSubmitters = []struct{ name, email, message string }{
	{"Ana Torres", "ana.torres@example.com", "Do you offer a self-hosted plan for a team of 12?"},
	{"Ben Okafor", "ben.okafor@example.com", "Can we get a demo of the status page feature?"},
	{"Chloe Martin", "chloe.martin@example.com", "We are moving from a hosted platform. Is there a migration guide?"},
	{"Dev Patel", "dev.patel@example.com", "Interested in the fleet features for our kiosk devices."},
	{"Elena Rossi", "elena.rossi@example.com", "Is there a discount for open source projects?"},
	{"Farid Haddad", "farid.haddad@example.com", "How are backups stored and encrypted?"},
}

func (s *seeder) pagesForms(site pagesSiteSpec, r *rng) error {
	contact := store.Form{
		ID: "form_demo_contact", AppName: site.name, Name: "contact", RedirectURL: "/thanks",
		HoneypotField: "website", IPSalt: r.hex(16), CreatedAt: s.at(historyDays, time.Hour),
	}
	news := store.Form{
		ID: "form_demo_newsletter", AppName: site.name, Name: "newsletter", RedirectURL: "/subscribed",
		HoneypotField: "website", IPSalt: r.hex(16), CreatedAt: s.at(historyDays, 2*time.Hour),
	}
	for _, f := range []store.Form{contact, news} {
		if err := s.db.CreateForm(s.ctx, f); err != nil {
			return fmt.Errorf("demo: create form %q: %w", f.Name, err)
		}
	}
	for i, p := range pagesSubmitters {
		sub := store.FormSubmission{
			FormID: contact.ID, Fields: map[string]string{"name": p.name, "email": p.email, "message": p.message},
			IPHash: r.hex(16), CreatedAt: s.at(12-i*2, time.Duration(8+r.intn(8))*time.Hour),
		}
		if _, err := s.db.AddFormSubmission(s.ctx, sub, 0); err != nil {
			return err
		}
		sub = store.FormSubmission{
			FormID: news.ID, Fields: map[string]string{"email": p.email}, IPHash: r.hex(16),
			CreatedAt: s.at(11-i*2, time.Duration(9+r.intn(8))*time.Hour),
		}
		if _, err := s.db.AddFormSubmission(s.ctx, sub, 0); err != nil {
			return err
		}
	}
	spam := store.FormSubmission{
		FormID: contact.ID, Fields: map[string]string{"name": "Cheap Deals", "email": "promo@example.com", "message": "Buy followers now"},
		Spam: true, SpamReason: "honeypot field filled", IPHash: r.hex(16), CreatedAt: s.at(3, 4*time.Hour),
	}
	_, err := s.db.AddFormSubmission(s.ctx, spam, 0)
	return err
}

// backupExtras adds what the Backups page shows beyond plain history: manual
// labels and a passed restore drill.
func (s *seeder) backupExtras() error {
	for id, label := range map[string]string{
		"bkp_pg-main_01":      "before schema migration",
		"bkp_pg-main_08":      "pre-upgrade to 16.4",
		"bkp_pg-analytics_05": "quarter close snapshot",
	} {
		if err := s.db.SetBackupLabel(s.ctx, id, label); err != nil {
			return err
		}
	}
	started := s.at(2, 5*time.Hour)
	d := store.BackupDrill{
		ID: "drill_demo_pg_main", BackupHistoryID: "bkp_pg-main_02", ResourceKind: store.BackupResourceKindDatabase,
		DatabaseName: "pg-main", TargetID: demoBackupTargetID, Trigger: store.DrillTriggerScheduled, StartedAt: s.stamp(started),
	}
	if err := s.db.StartBackupDrill(s.ctx, d); err != nil {
		return err
	}
	d.Status, d.Stage = store.DrillStatusPassed, "done"
	d.ObjectOK, d.ChecksumOK, d.RestoreOK, d.ContentOK = true, true, true, true
	d.Files, d.Bytes, d.DurationMS = 38, 214<<20, 96_000
	d.FinishedAt = s.stamp(started.Add(96 * time.Second))
	return s.db.FinishBackupDrill(s.ctx, d)
}

// previewSettings gives the storefront a real preview policy so the settings page is not empty.
func (s *seeder) previewSettings() error {
	err := s.db.SavePreviewAppSettings(s.ctx, store.PreviewAppSettings{
		AppName: "storefront-web", OnLimit: store.PreviewOnLimitEvictOldest, TTLHours: 72, MaxPreviews: 4,
		DatabaseStrategy: store.PreviewDatabaseStrategyFresh, RequireLabel: "preview", ExcludeServices: "worker,cache",
	})
	if err != nil {
		return fmt.Errorf("demo: save preview settings: %w", err)
	}
	at := s.stamp(s.at(1, 6*time.Hour))
	return s.db.SavePreviewEnvironment(s.ctx, store.PreviewEnvironment{
		ID: "prev_demo_storefront_212", AppName: "storefront-web", PRNumber: 212, PreviewAppID: "storefront-web-pr-212",
		Branch: "feat/cart-estimate", HeadSHA: "c0ffee1", Domain: "pr-212.shop." + s.opts.DomainSuffix,
		Status: store.PreviewStatusActive, CreatedAt: at, UpdatedAt: at,
	})
}
