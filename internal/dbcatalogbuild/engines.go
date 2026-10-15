package dbcatalogbuild

import "regexp"

// Spec describes where one engine's data comes from. Group 1 of StableRe and
// PreRe is the release line a tag belongs to.
type Spec struct {
	ID              string
	Repo            string
	EOLSlug         string
	ReleaseNotesURL string
	StableRe        *regexp.Regexp
	PreRe           *regexp.Regexp
	Lines           []string
}

// DockerHubBase is the OCI endpoint that serves Docker Hub repositories.
const DockerHubBase = "https://registry-1.docker.io"

// EOLBase is the endoflife.date API root.
const EOLBase = "https://endoflife.date/api"

var (
	reMinorPatch = regexp.MustCompile(`^(\d+\.\d+)\.\d+$`)
	reMinorRC    = regexp.MustCompile(`^(\d+\.\d+)(?:\.\d+)?-(?:rc|beta|alpha)\d+$`)
)

// Source returns the links published for the spec's engine.
func (s Spec) Source() Source {
	src := Source{TagsURL: DockerHubBase + "/v2/" + s.Repo + "/tags/list", ReleaseNotesURL: s.ReleaseNotesURL}
	if s.EOLSlug != "" {
		src.EOLURL = "https://endoflife.date/" + s.EOLSlug
	}
	return src
}

// SpecFor finds an engine's default spec.
func SpecFor(id string) (Spec, bool) {
	for _, s := range DefaultSpecs() {
		if s.ID == id {
			return s, true
		}
	}
	return Spec{}, false
}

// DefaultSpecs lists the engines the publisher covers. Engines whose images
// are not on Docker Hub (dragonfly, keydb) are left to the embedded catalog.
// Extending Lines is the one edit a new supported major needs.
func DefaultSpecs() []Spec {
	return []Spec{
		{
			ID: "postgres", Repo: "library/postgres", EOLSlug: "postgresql",
			ReleaseNotesURL: "https://www.postgresql.org/docs/release/",
			StableRe:        regexp.MustCompile(`^(\d+)\.\d+$`),
			PreRe:           regexp.MustCompile(`^(\d+)(?:rc|beta|alpha)\d+$`),
			Lines:           []string{"14", "15", "16", "17", "18"},
		},
		{
			ID: "redis", Repo: "library/redis", EOLSlug: "redis",
			ReleaseNotesURL: "https://github.com/redis/redis/releases",
			StableRe:        reMinorPatch, PreRe: reMinorRC,
			Lines: []string{"7.2", "7.4", "8.0", "8.2"},
		},
		{
			ID: "valkey", Repo: "valkey/valkey", EOLSlug: "valkey",
			ReleaseNotesURL: "https://github.com/valkey-io/valkey/releases",
			StableRe:        reMinorPatch, PreRe: reMinorRC,
			Lines: []string{"7.2", "8.0", "8.1", "9.0"},
		},
		{
			ID: "mysql", Repo: "library/mysql", EOLSlug: "mysql",
			ReleaseNotesURL: "https://dev.mysql.com/doc/relnotes/mysql/",
			StableRe:        reMinorPatch, PreRe: reMinorRC,
			Lines: []string{"8.0", "8.4"},
		},
		{
			ID: "mariadb", Repo: "library/mariadb", EOLSlug: "mariadb",
			ReleaseNotesURL: "https://mariadb.com/kb/en/release-notes/",
			StableRe:        reMinorPatch, PreRe: reMinorRC,
			Lines: []string{"10.6", "10.11", "11.4", "11.8"},
		},
		{
			ID: "mongodb", Repo: "library/mongo", EOLSlug: "mongodb",
			ReleaseNotesURL: "https://www.mongodb.com/docs/manual/release-notes/",
			StableRe:        reMinorPatch, PreRe: reMinorRC,
			Lines: []string{"6.0", "7.0", "8.0"},
		},
		{
			ID: "clickhouse", Repo: "clickhouse/clickhouse-server", EOLSlug: "clickhouse",
			ReleaseNotesURL: "https://clickhouse.com/docs/whats-new/changelog",
			StableRe:        regexp.MustCompile(`^(\d+\.\d+)$`),
			Lines:           []string{"24.8", "25.3", "25.8"},
		},
	}
}
