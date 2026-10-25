package catalog

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/compose"
)

var dataStoreImages = map[string]bool{
	"postgres": true, "postgis": true, "pgvector": true, "timescaledb": true, "mysql": true, "mariadb": true,
	"redis": true, "valkey": true, "keydb": true, "dragonfly": true, "memcached": true, "mongo": true,
	"clickhouse-server": true, "elasticsearch": true, "opensearch": true, "meilisearch": true, "typesense": true,
	"rabbitmq": true, "nats": true, "qdrant": true, "weaviate": true, "chroma": true, "milvus": true,
	"victoria-metrics": true, "influxdb": true, "couchdb": true, "etcd": true, "neo4j": true, "cassandra": true,
	"eclipse-mosquitto": true, "supertokens-postgresql": true,
}

// A database, cache, queue or search service that publishes a host port must
// gate it behind a generated credential, else it is open on every interface.
// Container-only ports (no host mapping) stay reachable only by the platform.
func TestTemplates_PublishedDataStorePortsRequireAuth(t *testing.T) {
	for _, tpl := range Templates {
		f, err := compose.Parse([]byte(tpl.Compose))
		if err != nil {
			continue
		}
		for name, svc := range f.Services {
			repo := svc.Image
			if i := strings.LastIndex(repo, "/"); i >= 0 {
				repo = repo[i+1:]
			}
			repo, _, _ = strings.Cut(repo, ":")
			if !dataStoreImages[repo] {
				continue
			}
			published := false
			for _, p := range svc.Ports {
				if p.HostPort > 0 {
					published = true
				}
			}
			if !published {
				continue
			}
			authed, disabled := false, false
			for k, v := range svc.Environment {
				if strings.Contains(strings.ToLower(k), "security.enabled") && strings.EqualFold(v, "false") {
					disabled = true
				}
				if secretLikeName.MatchString(k) && strings.Contains(v, "$SERVICE_") && !strings.Contains(v, "SERVICE_FQDN") {
					authed = true
				}
			}
			if !authed || disabled {
				t.Errorf("template %q service %q publishes a host port for %s without a generated credential; drop the host port or generate one", tpl.ID, name, repo)
			}
		}
	}
}
