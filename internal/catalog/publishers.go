package catalog

import (
	_ "embed"
	"fmt"
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
}

const publisherDateLayout = "2006-01-02"

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
	if p.Namespace == "" || strings.Count(p.Namespace, "/") != 1 {
		missing = append(missing, "namespace must be registry/org")
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
	} else if !exp.After(added) {
		missing = append(missing, "expires after reviewed_at")
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
		for _, o := range p.SourceOrgs {
			if strings.EqualFold(o, m[1]) {
				return true
			}
		}
	}
	return false
}
