package platformimport

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var secretKeyRe = regexp.MustCompile(`(?i)(^|_)(secret|password|passwd|pwd|token|api_?key|private_?key|credential|auth|dsn)($|_)|_key$|^database_url$|^redis_url$|^mongo(db)?_url$|^db_url$`)

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
	case strings.Contains(name, "redis"), strings.Contains(name, "valkey"):
		engine = "redis"
	}
	if m := regexp.MustCompile(`^v?(\d+(\.\d+)?)`).FindStringSubmatch(tag); m != nil {
		version = m[1]
	}
	return engine, version
}

func splitImageRef(image string) (repo, tag string) {
	if i := strings.LastIndex(image, ":"); i > strings.LastIndex(image, "/") {
		return image[:i], image[i+1:]
	}
	return image, ""
}

func gitURLFromRepo(repo, host string) string {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return ""
	}
	if strings.Contains(repo, "://") {
		return repo
	}
	if strings.HasPrefix(repo, "git@") {
		r := strings.TrimPrefix(repo, "git@")
		r = strings.Replace(r, ":", "/", 1)
		return "https://" + strings.TrimSuffix(r, ".git")
	}
	repo = strings.TrimSuffix(strings.Trim(repo, "/"), ".git")
	if first, _, _ := strings.Cut(repo, "/"); strings.Contains(first, ".") {
		return "https://" + repo
	}
	return "https://" + host + "/" + repo
}
