package platformimport

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/gitprovider/reporef"
)

var secretKeyRe = regexp.MustCompile(`(?i)(^|_)(secret|password|passwd|pwd|token|api_?key|private_?key|credential|auth|dsn)($|_)|_key$|^database_url$|^redis_url$|^mongo(db)?_url$|^db_url$|pass|secret|token|api_?key|credential|dsn`)

var userinfoValueRe = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://[^\s/@]*:[^\s/@]+@`)

// ValueLooksSecret reports whether a value embeds URL credentials, such as amqp://u:p@host.
func ValueLooksSecret(v string) bool {
	return userinfoValueRe.MatchString(v)
}

// LooksSecret reports whether an env var name suggests a secret value.
func LooksSecret(key string) bool {
	return secretKeyRe.MatchString(key)
}

// parseMemory parses Docker-style memory strings ("512m", "1g", "1073741824")
// into bytes. Zero, empty or unparseable input returns 0.
func parseMemory(s string) int64 {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || s == "0" {
		return 0
	}
	mult := int64(1)
	for suffix, m := range map[string]int64{"kb": 1 << 10, "mb": 1 << 20, "gb": 1 << 30, "k": 1 << 10, "m": 1 << 20, "g": 1 << 30, "b": 1} {
		if strings.HasSuffix(s, suffix) {
			mult = m
			s = strings.TrimSpace(strings.TrimSuffix(s, suffix))
			break
		}
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f <= 0 {
		return 0
	}
	return int64(f * float64(mult))
}

func parseCPUs(s string) int64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || f <= 0 {
		return 0
	}
	return int64(f * 1e9)
}

// hostFromURL extracts the bare host from "https://a.example.com/path" or
// "a.example.com". Wildcard and empty results return "".
func hostFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	h := strings.ToLower(u.Hostname())
	if h == "" || strings.Contains(h, "*") {
		return ""
	}
	return h
}

// splitHosts turns a comma-separated list of URLs or hosts into unique hosts.
func splitHosts(list string) []string {
	var out []string
	seen := map[string]bool{}
	for _, p := range strings.Split(list, ",") {
		if h := hostFromURL(p); h != "" && !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	return out
}

func firstPort(list string) int {
	for _, p := range strings.Split(list, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil && n > 0 && n < 65536 {
			return n
		}
	}
	return 0
}

// pgvectorTagPattern matches the Postgres major in pgvector image tags such as
// "pg17", "pg17-bookworm" and "0.8.0-pg17". Majors below 13 have no variant.
var pgvectorTagPattern = regexp.MustCompile(`(?:^|-)pg(1[3-9]|[2-9]\d)(?:-|$)`)

// engineFromImage guesses the database engine and a major version from an
// image reference such as "postgres:16-alpine".
func engineFromImage(image string) (engine, version string) {
	ref := image
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		ref = ref[i+1:]
	}
	name, tag := ref, ""
	if i := strings.Index(ref, ":"); i >= 0 {
		name, tag = ref[:i], ref[i+1:]
	}
	name = strings.ToLower(name)
	if name == "pgvector" {
		if m := pgvectorTagPattern.FindStringSubmatch(tag); m != nil {
			return "postgres", m[1] + "-pgvector"
		}
		return "postgres", ""
	}
	if name == "timescaledb" {
		if m := pgvectorTagPattern.FindStringSubmatch(tag); m != nil && m[1] >= "15" {
			return "postgres", m[1] + "-timescaledb"
		}
		return "postgres", ""
	}
	if name == "postgis" {
		if m := regexp.MustCompile(`^(1[3-9]|[2-9]\d)(?:-|$)`).FindStringSubmatch(tag); m != nil {
			return "postgres", m[1] + "-postgis"
		}
		return "postgres", ""
	}
	switch {
	case strings.Contains(name, "postgres"), strings.Contains(name, "postgis"):
		engine = "postgres"
	case strings.Contains(name, "mariadb"):
		engine = "mariadb"
	case strings.Contains(name, "mysql"):
		engine = "mysql"
	case strings.Contains(name, "mongo"):
		engine = "mongodb"
	case strings.Contains(name, "keydb"):
		engine = "keydb"
	case strings.Contains(name, "dragonfly"):
		engine = "dragonfly"
	case strings.Contains(name, "clickhouse"):
		engine = "clickhouse"
	case strings.Contains(name, "valkey"):
		engine = "valkey"
	case strings.Contains(name, "redis"):
		engine = "redis"
	case strings.Contains(name, "meilisearch"):
		engine = "meilisearch"
	case strings.Contains(name, "qdrant"):
		engine = "qdrant"
	case strings.Contains(name, "typesense"):
		engine = "typesense"
	case strings.Contains(name, "memcached"):
		engine = "memcached"
	}
	if engine == "dragonfly" {
		// Dragonfly publishes only full vX.Y.Z tags, so a shortened one would not pull.
		if dragonflyTagPattern.MatchString(tag) {
			return engine, tag
		}
		return engine, ""
	}
	if engine == "keydb" {
		// The managed engine runs eqalpha/keydb with the version as its tag, so only an exact tag of that image is kept.
		repo := strings.TrimPrefix(strings.ToLower(strings.TrimSuffix(image, ":"+tag)), "docker.io/")
		if repo == "eqalpha/keydb" && keydbTagPattern.MatchString(tag) {
			return engine, tag
		}
		return engine, ""
	}
	if m := regexp.MustCompile(`^v?(\d+(\.\d+)?)`).FindStringSubmatch(tag); m != nil {
		version = m[1]
		if engine == "meilisearch" || engine == "qdrant" {
			// Their tags keep the "v" prefix.
			version = "v" + version
		}
	}
	return engine, version
}

var (
	dragonflyTagPattern = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
	keydbTagPattern     = regexp.MustCompile(`^(?:[a-z0-9]+(?:_[a-z0-9]+)*_)?v?\d+\.\d+\.\d+$`)
)

func splitImageRef(image string) (repo, tag string) {
	if i := strings.LastIndex(image, ":"); i > strings.LastIndex(image, "/") {
		return image[:i], image[i+1:]
	}
	return image, ""
}

// sourceRepoHost is where an "owner/repo" repository field without a host lives.
const sourceRepoHost = "github.com"

// gitURLFromRepo maps a source's repository field to a clone URL: http(s)
// URLs pass through, ssh, scp style and "owner/repo" forms become https,
// and anything unreadable is returned as is for the importer to report.
func gitURLFromRepo(repo string) string {
	repo = strings.TrimSpace(repo)
	r, err := reporef.ParseWithHost(repo, sourceRepoHost)
	if err != nil || (r.IsHTTP() && strings.Contains(repo, "://")) {
		return repo
	}
	return r.CloneURL()
}

// setGitURL records the app's clone URL and notes an ssh source mapped to https.
func (a *App) setGitURL(repo string) {
	a.GitURL = gitURLFromRepo(repo)
	if r, err := reporef.Parse(repo); err == nil && !r.IsHTTP() {
		a.Notes = append(a.Notes, Note{Reason: "the source cloned the repository over ssh; it is imported as " + a.GitURL,
			Manual: "connect a git provider that can read the repository, or add a deploy key after import"})
	}
	a.scrubGitURL()
}

// sensitiveRepoQueryKeys are query parameters that commonly carry a git token.
var sensitiveRepoQueryKeys = map[string]struct{}{
	"access_token": {}, "token": {}, "private_token": {}, "oauth_token": {},
	"password": {}, "secret": {}, "key": {}, "api_key": {}, "apikey": {}, "auth": {},
}

// DisplayRepoURL returns raw without userinfo, secret query values or fragment,
// and whether anything was removed. An unparseable URL form yields "" so the
// original is never echoed.
func DisplayRepoURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		// scp-style git@host:path carries a login name, not a secret, unless it has a password.
		if at := strings.Index(raw, "@"); at > 0 && strings.Contains(raw[:at], ":") {
			return "", true
		}
		return raw, false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", true
	}
	changed := u.User != nil || u.Fragment != ""
	u.User = nil
	u.Fragment = ""
	u.RawFragment = ""
	if u.RawQuery != "" {
		q := u.Query()
		for k := range q {
			if _, bad := sensitiveRepoQueryKeys[strings.ToLower(k)]; bad {
				q.Del(k)
				changed = true
			}
		}
		u.RawQuery = q.Encode()
	}
	return u.String(), changed
}

// scrubGitURL drops inline credentials from the app's git URL and notes it,
// so plans and reports never carry a token.
func (a *App) scrubGitURL() {
	clean, changed := DisplayRepoURL(a.GitURL)
	if !changed {
		return
	}
	a.GitURL = clean
	a.Notes = append(a.Notes, Note{Reason: "a credential was removed from the repository URL", Manual: "supply it with the git credential helper or a deploy key instead of inline"})
}
