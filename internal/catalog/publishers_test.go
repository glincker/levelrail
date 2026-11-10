package catalog

import (
	"testing"
	"time"
)

func TestPublishersAreComplete(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range Publishers() {
		if err := p.Validate(); err != nil {
			t.Error(err)
		}
		if seen[p.Namespace] {
			t.Errorf("duplicate namespace %s", p.Namespace)
		}
		seen[p.Namespace] = true
	}
}

func TestPublishersNotExpired(t *testing.T) {
	for _, p := range Publishers() {
		if p.Expired(time.Now()) {
			t.Errorf("publisher %s expired on %s, renew the evidence or remove it", p.Namespace, p.Expires)
		}
	}
}

func TestPublisherValidateRejectsGaps(t *testing.T) {
	ok := Publisher{Namespace: "docker.io/acme", SourceOrgs: []string{"acme"}, Evidence: "https://github.com/acme/app", ReviewedBy: "me", ReviewedAt: "2026-01-01", Expires: "2027-01-01"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid entry rejected: %v", err)
	}
	for name, mut := range map[string]func(*Publisher){
		"no evidence": func(p *Publisher) { p.Evidence = "" },
		"http only":   func(p *Publisher) { p.Evidence = "http://example.com/x" },
		"no reviewer": func(p *Publisher) { p.ReviewedBy = " " },
		"no orgs":     func(p *Publisher) { p.SourceOrgs = nil },
		"bad expiry":  func(p *Publisher) { p.Expires = "soon" },
		"bad order":   func(p *Publisher) { p.Expires = "2025-01-01" },
		"no registry": func(p *Publisher) { p.Namespace = "acme" },
	} {
		p := ok
		mut(&p)
		if p.Validate() == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestExpiredPublisherStopsValidating(t *testing.T) {
	old := publishers
	defer func() { publishers = old }()
	publishers = []Publisher{{Namespace: "docker.io/acme", SourceOrgs: []string{"Acme-Inc"}, Evidence: "https://github.com/acme/app", ReviewedBy: "me", ReviewedAt: "2026-01-01", Expires: "2026-06-01"}}
	repo := "https://github.com/acme-inc/app"
	if ok := verifiedPublisherFor("acme/app", repo, time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)); !ok {
		t.Fatal("entry should validate before expiry")
	}
	if ok := verifiedPublisherFor("acme/app", repo, time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)); ok {
		t.Fatal("expired entry must not validate")
	}
	if ok := verifiedPublisherFor("acme/app", "https://github.com/other/app", time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)); ok {
		t.Fatal("wrong source org must not validate")
	}
}

func TestImageNamespace(t *testing.T) {
	for in, want := range map[string]string{
		"prom/node-exporter": "docker.io/prom", "docker.io/prom/x": "docker.io/prom",
		"ghcr.io/Org/app": "ghcr.io/org", "postgres": "", "quay.io/a/b/c": "quay.io/a",
	} {
		if got := imageNamespace(in); got != want {
			t.Errorf("imageNamespace(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestVerifiedPublisherClass(t *testing.T) {
	if got := ImageSourceClass("prom/node-exporter:v1.12.1", "https://github.com/prometheus/node_exporter"); got != SourceVerifiedPublisher {
		t.Fatalf("class = %q", got)
	}
	if got := ImageSourceClass("prom/node-exporter:v1.12.1", "https://github.com/evil/node_exporter"); got != SourceCommunity {
		t.Fatalf("wrong org class = %q", got)
	}
}
