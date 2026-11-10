package catalog

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
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

func TestSpoofedRegistryHostIsCommunity(t *testing.T) {
	repo := "https://github.com/acme/demo"
	for ref, want := range map[string]string{
		"acme/demo:1.2.3":               SourceUpstream,
		"docker.io/acme/demo:1.2.3":     SourceUpstream,
		"ghcr.io/acme/demo:1.2.3":       SourceUpstream,
		"quay.io/acme/demo:1.2.3":       SourceUpstream,
		"quay.io/other/demo:1.2.3":      SourceCommunity,
		"evil.example.com/acme/demo:1":  SourceCommunity,
		"localhost:5000/acme/demo:1":    SourceCommunity,
		"ghcr.io/other/demo:1.2.3":      SourceCommunity,
		"registry.example.io/acme/x:v1": SourceCommunity,
	} {
		if got := ImageSourceClass(ref, repo); got != want {
			t.Errorf("ImageSourceClass(%q) = %q, want %q", ref, got, want)
		}
	}
}

func TestExceptionsAreKeyedByImageRepo(t *testing.T) {
	good := strings.Replace(testCompose, "nginx:alpine", "erikvl87/languagetool:6.8", 1)
	lt := strings.Replace(submissionManifest, "id: demo", "id: languagetool", 1)
	run := func(compose string) error {
		fsys := fstest.MapFS{
			"m/languagetool/template.yaml": {Data: []byte(lt)},
			"m/languagetool/compose.yaml":  {Data: []byte(compose)},
		}
		return ValidateSubmissions(fsys, "m", nil, nil)
	}
	if err := run(good); err != nil {
		t.Fatalf("exact repo should keep its exception: %v", err)
	}
	for _, img := range []string{"someone/else:1.2", "evil.example.com/erikvl87/languagetool:6.8"} {
		other := strings.Replace(testCompose, "nginx:alpine", img, 1)
		if err := run(other); err == nil {
			t.Errorf("image %s must not inherit the languagetool exception through the template id", img)
		}
	}
}

func TestPersonalAccountIsRepoScoped(t *testing.T) {
	old, oldClock := publishers, publisherClock
	defer func() { publishers, publisherClock = old, oldClock }()
	publisherClock = func() time.Time { return time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC) }
	publishers = []Publisher{{Namespace: "docker.io/someone", SourceOrgs: []string{"acme"}, Evidence: "https://github.com/acme/app", ReviewedBy: "me", ReviewedAt: "2026-11-10", Expires: "2027-02-01", Personal: true, Repos: []string{"someone/app"}}}
	repo := "https://github.com/acme/app"
	if got := ImageSourceClass("someone/app:1.0.0", repo); got != SourceVerifiedPublisher {
		t.Fatalf("listed repo class = %q", got)
	}
	if got := ImageSourceClass("someone/other:1.0.0", repo); got != SourceCommunity {
		t.Fatalf("unlisted repo class = %q", got)
	}
	publishers[0].Repos = nil
	if publishers[0].Validate() == nil {
		t.Fatal("personal account without repos must not validate")
	}
}

func TestPublisherWindowAndNamespace(t *testing.T) {
	oldClock := publisherClock
	defer func() { publisherClock = oldClock }()
	publisherClock = func() time.Time { return time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC) }
	ok := Publisher{Namespace: "docker.io/acme", SourceOrgs: []string{"acme"}, Evidence: "https://github.com/acme/app", ReviewedBy: "me", ReviewedAt: "2026-11-10", Expires: "2027-11-10"}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mut := range map[string]func(*Publisher){
		"over 400 days":  func(p *Publisher) { p.Expires = "2027-12-20" },
		"future review":  func(p *Publisher) { p.ReviewedAt = "2026-12-01"; p.Expires = "2027-06-01" },
		"empty org":      func(p *Publisher) { p.Namespace = "docker.io/" },
		"uppercase":      func(p *Publisher) { p.Namespace = "docker.io/Acme" },
		"nested path":    func(p *Publisher) { p.Namespace = "docker.io/acme/app" },
		"personal empty": func(p *Publisher) { p.Personal = true },
	} {
		p := ok
		mut(&p)
		if p.Validate() == nil {
			t.Errorf("%s: want error", name)
		}
	}
	p := ok
	p.Namespace = "registry.example.com:5000/acme"
	if err := p.Validate(); err != nil {
		t.Errorf("host with port rejected: %v", err)
	}
}

func TestHealthcheckFourXXRule(t *testing.T) {
	good := strings.Replace(testCompose, "nginx:alpine", "acme/demo:1.2.3", 1)
	base := strings.Replace(submissionManifest, "healthcheck:\n  service: web\n  port: 80\n  path: /\n", "", 1)
	for _, tc := range []struct {
		code int
		ok   bool
	}{{404, true}, {405, true}, {400, false}, {418, false}, {429, false}} {
		hc := fmt.Sprintf("healthcheck:\n  service: web\n  port: 80\n  path: /\n  expect: %d\n  reason: documented\n", tc.code)
		if err := submissionErr(base+hc, good); (err == nil) != tc.ok {
			t.Errorf("expect %d: err = %v, want ok=%v", tc.code, err, tc.ok)
		}
	}
}

func TestPublishersExpiringSoon(t *testing.T) {
	for _, p := range Publishers() {
		exp, err := time.Parse(publisherDateLayout, p.Expires)
		if err == nil && time.Until(exp) < 30*24*time.Hour {
			t.Logf("publisher %s expires %s, renew the evidence soon", p.Namespace, p.Expires)
		}
	}
}
