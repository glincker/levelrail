package catalog

import (
	_ "embed"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed publishers.yaml
var publishersYAML []byte

// SourceVerifiedPublisher marks an image whose registry namespace is on the
// reviewed allowlist for the template's source org.
const SourceVerifiedPublisher = "verified_publisher"

// Publisher is one reviewed image namespace, for example docker.io/prom.
type Publisher struct {
	Namespace  string   `yaml:"namespace"`
	SourceOrgs []string `yaml:"source_orgs"`
	Evidence   string   `yaml:"evidence"`
	ReviewedBy string   `yaml:"reviewed_by"`
	ReviewedAt string   `yaml:"reviewed_at"`
	Expires    string   `yaml:"expires"`
	Note       string   `yaml:"note,omitempty"`
	// Personal marks an individual's account; it must list the exact repos it covers.
	Personal bool     `yaml:"personal,omitempty"`
	Repos    []string `yaml:"repos,omitempty"`
}

const publisherDateLayout = "2006-01-02"

// maxPublisherReview bounds how long one review can vouch for a namespace.
const maxPublisherReview = 400 * 24 * time.Hour

var publisherNamespacePattern = regexp.MustCompile(`^[a-z0-9.-]+(:\d+)?/[a-z0-9][a-z0-9._-]*$`)

var publishers = loadPublishers()

// publisherClock is swapped in tests so expiry is deterministic.
var publisherClock = time.Now

func loadPublishers() []Publisher {
	var ps []Publisher
	if err := yaml.Unmarshal(publishersYAML, &ps); err != nil {
		panic("catalog: publishers.yaml: " + err.Error())
	}
	return ps
}

// Publishers returns the committed allowlist, expired entries included.
func Publishers() []Publisher { return publishers }

// Validate reports why an entry cannot be trusted: missing proof, missing
// reviewer, bad dates or an https-less evidence link.
func (p Publisher) Validate() error {
	var missing []string
	if !publisherNamespacePattern.MatchString(p.Namespace) {
		missing = append(missing, "namespace must be registry/org in lowercase")
	}
	if p.Personal && len(p.Repos) == 0 {
		missing = append(missing, "repos (required for personal accounts)")
	}
	if len(p.SourceOrgs) == 0 {
		missing = append(missing, "source_orgs")
	}
	if !httpsURLPattern.MatchString(p.Evidence) {
		missing = append(missing, "evidence https URL")
	}
	if strings.TrimSpace(p.ReviewedBy) == "" {
		missing = append(missing, "reviewed_by")
	}
	added, err1 := time.Parse(publisherDateLayout, p.ReviewedAt)
	exp, err2 := time.Parse(publisherDateLayout, p.Expires)
	if err1 != nil || err2 != nil {
		missing = append(missing, "reviewed_at and expires as YYYY-MM-DD")
	} else {
		if !exp.After(added) {
			missing = append(missing, "expires after reviewed_at")
		}
		if exp.Sub(added) > maxPublisherReview {
			missing = append(missing, "expires within 400 days of reviewed_at")
		}
		if added.After(publisherClock().UTC().AddDate(0, 0, 1)) {
			missing = append(missing, "reviewed_at not in the future")
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("publisher %q: %s", p.Namespace, strings.Join(missing, ", "))
	}
	return nil
}

// Expired reports whether the entry no longer validates at the given time.
func (p Publisher) Expired(now time.Time) bool {
	exp, err := time.Parse(publisherDateLayout, p.Expires)
	return err != nil || !now.Before(exp.AddDate(0, 0, 1))
}

// imageNamespace returns registry/org for an image name without tag.
func imageNamespace(img string) string {
	parts := strings.Split(strings.TrimPrefix(img, "docker.io/"), "/")
	registry := "docker.io"
	if strings.Contains(parts[0], ".") && len(parts) > 2 {
		registry, parts = parts[0], parts[1:]
	}
	if len(parts) < 2 {
		return ""
	}
	return strings.ToLower(registry + "/" + parts[0])
}

// verifiedPublisherFor finds a valid, unexpired entry covering the image
// namespace and the source repo org.
func verifiedPublisherFor(img, sourceRepo string, now time.Time) bool {
	ns := imageNamespace(img)
	m := githubRepoPattern.FindStringSubmatch(sourceRepo)
	if ns == "" || m == nil {
		return false
	}
	for _, p := range publishers {
		if strings.ToLower(p.Namespace) != ns || p.Validate() != nil || p.Expired(now) {
			continue
		}
		if len(p.Repos) > 0 && !repoListed(p.Repos, img) {
			continue
		}
		for _, o := range p.SourceOrgs {
			if strings.EqualFold(o, m[1]) {
				return true
			}
		}
	}
	return false
}

// repoListed reports whether the image repo path (registry removed) is in repos.
func repoListed(repos []string, img string) bool {
	_, path := splitRegistry(img)
	for _, r := range repos {
		if strings.EqualFold(r, path) {
			return true
		}
	}
	return false
}
