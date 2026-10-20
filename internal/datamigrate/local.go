package datamigrate

import (
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// Docker's built-in network names, none of which has embedded DNS or can be
// joined to reach a container that lives on a user-defined network.
const (
	defaultBridgeNetwork = "bridge"
	hostNetwork          = "host"
	noneNetwork          = "none"
)

// LocalSource is a database container on this daemon, resolved to how the
// helper must reach it: join Network and connect to Host:Port.
type LocalSource struct {
	Container string `json:"container"`
	Image     string `json:"image"`
	Engine    string `json:"engine"`
	Running   bool   `json:"running"`
	Network   string `json:"network,omitempty"`
	Host      string `json:"host,omitempty"`
	Port      int    `json:"port,omitempty"`
	// Problem is why this container cannot be used, empty when it can.
	Problem string `json:"problem,omitempty"`
}

// EngineFromImage maps an image reference to an engine id, "" when it is not
// a database this package can read.
func EngineFromImage(image string) string {
	repo := strings.ToLower(image)
	if i := strings.LastIndex(repo, ":"); i > strings.LastIndex(repo, "/") {
		repo = repo[:i]
	}
	base := repo[strings.LastIndex(repo, "/")+1:]
	switch {
	case strings.Contains(base, "mariadb"):
		return EngineMariaDB
	case strings.Contains(base, "mysql"), strings.Contains(base, "percona"):
		return EngineMySQL
	case strings.Contains(base, "mongo"):
		return EngineMongoDB
	case strings.Contains(base, "postgres"), strings.Contains(base, "pgvector"),
		strings.Contains(base, "postgis"), strings.Contains(base, "timescale"), strings.Contains(base, "supabase"):
		return EnginePostgres
	}
	return ""
}

// SelectLocalSource decides which network the helper joins and what address it
// uses. A user-defined network wins and the container name is used (embedded
// DNS). Only the default bridge falls back to the internal IP. It never uses a
// published host address: Docker's bridge isolation drops that traffic.
func SelectLocalSource(c docker.LocalContainer) LocalSource {
	engine := EngineFromImage(c.Image)
	s := LocalSource{Container: c.Name, Image: c.Image, Engine: engine, Running: c.Running}
	if engine == "" {
		s.Problem = "not a database image this migration can read"
		return s
	}
	if !c.Running {
		s.Problem = "the container is not running, start it first"
		return s
	}
	s.Port = DefaultPort(engine)
	for _, p := range c.Ports {
		if p == DefaultPort(engine) {
			s.Port = p
		}
	}
	if len(c.Ports) > 0 && !containsInt(c.Ports, s.Port) {
		s.Port = c.Ports[0]
	}
	var bridgeIP string
	for _, n := range c.Networks {
		switch n.Name {
		case defaultBridgeNetwork:
			bridgeIP = n.IP
		case hostNetwork, noneNetwork:
		default:
			s.Network, s.Host = n.Name, c.Name
			return s
		}
	}
	if bridgeIP != "" {
		s.Network, s.Host = defaultBridgeNetwork, bridgeIP
		return s
	}
	s.Problem = fmt.Sprintf("the container is on the %q network only, connect to it by its host address instead", hostNetwork)
	return s
}

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// ExplainReachFailure turns a connection failure into a plain explanation,
// naming the Docker network isolation case when the source is not a local
// container. The raw error is always kept in full after it.
func ExplainReachFailure(raw string, local bool) string {
	lower := strings.ToLower(raw)
	unreachable := strings.Contains(lower, "timeout") || strings.Contains(lower, "timed out") ||
		strings.Contains(lower, "could not connect") || strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "no route") || strings.Contains(lower, "can't connect")
	if local || !unreachable {
		return raw
	}
	return "The migration helper could not reach the source. If the source is a container on this same host, " +
		"Docker's network isolation drops traffic from the helper to another network's containers when it goes through the host address. " +
		"Pick the container from the local sources list so the helper joins its network, or use an address the helper can route to. " +
		"Details: " + raw
}
