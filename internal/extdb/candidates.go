package extdb

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
	appspec "github.com/GLINCKER/levelrail/internal/spec"
)

// imageEngines maps the last path segment of an image repository to an engine.
var imageEngines = map[string]string{
	"postgres":                 EnginePostgres,
	"pgvector":                 EnginePostgres,
	"postgis":                  EnginePostgres,
	"timescaledb":              EnginePostgres,
	"postgresql":               EnginePostgres,
	"mysql":                    EngineMySQL,
	"mysql-server":             EngineMySQL,
	"mariadb":                  EngineMariaDB,
	"mongo":                    EngineMongoDB,
	"mongodb":                  EngineMongoDB,
	"mongodb-community-server": EngineMongoDB,
	"redis":                    EngineRedis,
	"valkey":                   EngineRedis,
	"redis-stack-server":       EngineRedis,
}

// EngineForImage reports which database engine an image runs, or "" when it
// is not one of the supported engines.
func EngineForImage(image string) string {
	ref := image
	if i := strings.Index(ref, "@"); i >= 0 {
		ref = ref[:i]
	}
	if i := strings.LastIndex(ref, ":"); i >= 0 && !strings.Contains(ref[i:], "/") {
		ref = ref[:i]
	}
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		ref = ref[i+1:]
	}
	return imageEngines[strings.ToLower(ref)]
}

// Candidate is one running database container that could be adopted.
type Candidate struct {
	ContainerID   string   `json:"container_id"`
	Container     string   `json:"container"`
	Image         string   `json:"image"`
	Engine        string   `json:"engine"`
	Port          int      `json:"port"`
	SuggestedHost string   `json:"suggested_host,omitempty"`
	Network       string   `json:"network,omitempty"`
	Networks      []string `json:"networks,omitempty"`
	PublishedPort int      `json:"published_port,omitempty"`
	SuggestedUser string   `json:"suggested_user,omitempty"`
	Note          string   `json:"note,omitempty"`
}

// ListCandidates finds running database containers on rt. Containers this
// platform created itself carry its instance label and are never offered.
// It only reads container metadata.
func ListCandidates(ctx context.Context, rt docker.Runtime) ([]Candidate, error) {
	states, err := rt.ListByPrefix(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("extdb: list containers: %w", err)
	}
	var out []Candidate
	for _, st := range states {
		if !st.Running {
			continue
		}
		if _, managed := st.Labels[appspec.InstanceLabelKey]; managed {
			continue
		}
		engine := EngineForImage(st.Image)
		if engine == "" {
			continue
		}
		out = append(out, candidateFrom(st, engine))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Container < out[j].Container })
	return out, nil
}

func candidateFrom(st docker.ContainerState, engine string) Candidate {
	c := Candidate{
		ContainerID: st.ID, Container: st.Name, Image: st.Image, Engine: engine,
		Port: DefaultPort(engine), SuggestedUser: suggestedUser(engine),
	}
	for _, b := range st.Ports {
		if b.ContainerPort == c.Port && b.HostPort > 0 {
			c.PublishedPort = b.HostPort
			break
		}
	}
	var bridgeIP string
	for _, n := range st.Networks {
		c.Networks = append(c.Networks, n.Name)
		switch {
		case n.Name == "bridge":
			bridgeIP = n.IPAddress
		case !builtinNetworks[n.Name] && c.Network == "":
			c.Network = n.Name
			c.SuggestedHost = st.Name
		}
	}
	switch {
	case c.SuggestedHost != "":
	case bridgeIP != "":
		c.SuggestedHost = bridgeIP
		c.Note = "on the default bridge network, the address can change when the container restarts"
	case c.PublishedPort > 0:
		c.Note = "no shared network found, use the host's address and port " + fmt.Sprint(c.PublishedPort)
	default:
		c.Note = "network details are not available for this node, enter the address manually"
	}
	return c
}

func suggestedUser(engine string) string {
	switch engine {
	case EnginePostgres:
		return "postgres"
	case EngineMySQL, EngineMariaDB:
		return "root"
	}
	return ""
}
