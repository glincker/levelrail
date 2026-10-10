package platformimport

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// DockerSource maps a `docker inspect` snapshot of every container on a host
// into the neutral model. It is read-only and needs no credential for the
// platform that launched the containers, so it works when only the Docker
// host is reachable (no Coolify token, no Dokploy key).
type DockerSource struct {
	Data []byte
}

type dockerContainer struct {
	ID      string `json:"Id"`
	Name    string `json:"Name"`
	ImageID string `json:"Image"`
	Config  struct {
		Image       string              `json:"Image"`
		Env         []string            `json:"Env"`
		Labels      map[string]string   `json:"Labels"`
		Exposed     map[string]struct{} `json:"ExposedPorts"`
		Healthcheck *struct {
			Test     []string `json:"Test"`
			Interval int64    `json:"Interval"`
			Timeout  int64    `json:"Timeout"`
			Retries  int      `json:"Retries"`
		} `json:"Healthcheck"`
	} `json:"Config"`
	State struct {
		Running bool `json:"Running"`
	} `json:"State"`
	HostConfig struct {
		Memory   int64 `json:"Memory"`
		NanoCPUs int64 `json:"NanoCpus"`
	} `json:"HostConfig"`
	Mounts []struct {
		Type        string `json:"Type"`
		Name        string `json:"Name"`
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
		RW          bool   `json:"RW"`
	} `json:"Mounts"`
	Created string `json:"Created"`
}

var (
	traefikRuleKey = regexp.MustCompile(`^traefik\.http\.routers\.[^.]+\.rule$`)
	traefikPortKey = regexp.MustCompile(`^traefik\.http\.services\.[^.]+\.loadbalancer\.server\.port$`)
	traefikHostRe  = regexp.MustCompile("Host\\(`([^`]+)`\\)")
	healthURLRe    = regexp.MustCompile(`https?://[^/\s'"]+(/[^\s'"]*)?`)
	builtImageRe   = regexp.MustCompile(`^[a-z0-9]{20,}$`)
	commitTagRe    = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// dockerPlatformEnv are variables every image or runtime injects; they say
// nothing about the app's own configuration.
var dockerPlatformEnv = map[string]bool{
	"PATH": true, "HOSTNAME": true, "HOME": true, "TERM": true, "LANG": true,
	"PWD": true, "SHLVL": true,
}

const dockerSocketPath = "/var/run/docker.sock"

func dockerInfraContainer(c dockerContainer, name string) bool {
	img := strings.ToLower(c.Config.Image)
	switch {
	case strings.HasPrefix(name, "coolify"), strings.HasPrefix(name, "levelrail"):
		return true
	case strings.Contains(img, "coollabsio/coolify"), strings.Contains(img, "levelrail"):
		return true
	case c.Config.Labels["coolify.type"] == "proxy", c.Config.Labels["coolify.type"] == "helper":
		return true
	}
	return false
}

// dockerGroupKey ties containers of one logical resource together: the
// source platform's own id when labelled, else the compose service, else the
// container name.
func dockerGroupKey(c dockerContainer, name string) string {
	l := c.Config.Labels
	for _, k := range []string{"coolify.applicationId", "coolify.serviceId", "coolify.databaseId"} {
		if v := l[k]; v != "" {
			return "coolify:" + v
		}
	}
	if p, s := l["com.docker.compose.project"], l["com.docker.compose.service"]; p != "" && s != "" {
		return "compose:" + p + "/" + s
	}
	return "name:" + name
}

func dockerDisplayName(c dockerContainer, name string) string {
	l := c.Config.Labels
	for _, v := range []string{l["coolify.resourceName"], l["com.docker.compose.service"], name} {
		if v != "" {
			return v
		}
	}
	return c.ID
}

// Discover parses the snapshot, a JSON array as printed by
// `docker inspect $(docker ps -aq)`.
func (s DockerSource) Discover(_ context.Context) (*Discovery, error) {
	var all []dockerContainer
	if err := json.Unmarshal(s.Data, &all); err != nil {
		return nil, fmt.Errorf("parse docker inspect output (expected a JSON array): %w", err)
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("the docker inspect snapshot has no containers")
	}
	groups := map[string][]dockerContainer{}
	var order []string
	for _, c := range all {
		name := strings.TrimPrefix(c.Name, "/")
		if dockerInfraContainer(c, name) {
			continue
		}
		k := dockerGroupKey(c, name)
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], c)
	}
	sort.Strings(order)
	d := &Discovery{Platform: Docker}
	projects := map[string]bool{}
	for _, k := range order {
		members := groups[k]
		best, running := pickDockerMember(members)
		name := dockerDisplayName(best, strings.TrimPrefix(best.Name, "/"))
		l := best.Config.Labels
		project := l["coolify.projectName"]
		if project != "" && !projects[project] {
			projects[project] = true
			d.Projects = append(d.Projects, project)
		}
		if l["coolify.type"] == "service" {
			d.Unsupported = append(d.Unsupported, Unsupported{Kind: "service", SourceID: k, Name: name, Project: project,
				Environment: l["coolify.environmentName"], Source: "service " + best.Config.Image,
				Reason: "one-click service stacks are Docker Compose projects",
				Manual: "export the service's compose file from the source host and deploy it with the compose deploy command"})
			continue
		}
		domains, port := dockerRouting(best)
		if engine, ver := engineFromImage(best.Config.Image); engine != "" && len(domains) == 0 {
			d.Databases = append(d.Databases, Database{SourceID: k, Name: name, Project: project, Engine: engine, Version: ver,
				Environment: l["coolify.environmentName"], InternalHost: strings.TrimPrefix(best.Name, "/")})
			continue
		}
		d.Apps = append(d.Apps, dockerApp(k, name, project, best, domains, port, running))
	}
	sort.Strings(d.Projects)
	return d, nil
}

// pickDockerMember prefers a running container, then the newest, and
// reports how many members run (the replica count).
func pickDockerMember(members []dockerContainer) (dockerContainer, int) {
	sort.SliceStable(members, func(i, j int) bool {
		if members[i].State.Running != members[j].State.Running {
			return members[i].State.Running
		}
		return members[i].Created > members[j].Created
	})
	running := 0
	for _, m := range members {
		if m.State.Running {
			running++
		}
	}
	return members[0], running
}

func dockerRouting(c dockerContainer) (domains []string, port int) {
	seen := map[string]bool{}
	keys := make([]string, 0, len(c.Config.Labels))
	for k := range c.Config.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := c.Config.Labels[k]
		switch {
		case traefikRuleKey.MatchString(k):
			for _, m := range traefikHostRe.FindAllStringSubmatch(v, -1) {
				h := strings.ToLower(m[1])
				if !seen[h] {
					seen[h] = true
					domains = append(domains, h)
				}
			}
		case traefikPortKey.MatchString(k) && port == 0:
			port = firstPort(v)
		}
	}
	if port == 0 {
		low := 0
		for p := range c.Config.Exposed {
			if n := firstPort(strings.TrimSuffix(p, "/tcp")); n > 0 && (low == 0 || n < low) && !strings.HasSuffix(p, "/udp") {
				low = n
			}
		}
		port = low
	}
	return domains, port
}

func dockerApp(id, name, project string, c dockerContainer, domains []string, port, running int) App {
	app := App{SourceID: id, Name: name, Project: project, Environment: c.Config.Labels["coolify.environmentName"],
		Kind: SourceImage, Image: c.Config.Image, Domains: domains, Port: port, Replicas: running,
		MemoryBytes: c.HostConfig.Memory, NanoCPUs: c.HostConfig.NanoCPUs, ImageID: c.ImageID}
	if app.Replicas == 0 {
		app.Replicas = 1
		app.Notes = append(app.Notes, Note{Reason: "no container of this app was running in the snapshot", Manual: "confirm it is meant to run before cutover"})
	}
	if dockerBuiltLocally(c.Config.Image) {
		app.HostBuilt = true
		app.Notes = append(app.Notes, Note{Reason: "the image was built on the source host and is not in a registry",
			Manual: "move it with the import's Move images step before verify, or by hand: docker save " + c.Config.Image + " | ssh <target> docker load"})
	}
	dropped := 0
	for _, kv := range c.Config.Env {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || dockerPlatformEnv[k] {
			continue
		}
		if strings.HasPrefix(k, "COOLIFY_") {
			dropped++
			continue
		}
		app.Env = append(app.Env, Env{Key: k, Value: v, Secret: LooksSecret(k)})
	}
	if dropped > 0 {
		app.Notes = append(app.Notes, Note{Reason: fmt.Sprintf("%d platform-injected COOLIFY_* variables were left out", dropped),
			Manual: "set any your app reads (for example its public URL) after import"})
	}
	for _, m := range c.Mounts {
		if m.Source == dockerSocketPath || m.Destination == dockerSocketPath {
			app.Notes = append(app.Notes, Note{Reason: "the container mounts the Docker socket, which is not imported",
				Manual: "grant that access deliberately if the app truly needs it"})
			continue
		}
		v := Volume{ContainerPath: m.Destination, ReadOnly: !m.RW}
		if m.Type == "bind" {
			v.HostPath = m.Source
		} else {
			v.Name = m.Name
		}
		app.Volumes = append(app.Volumes, v)
	}
	if hc := c.Config.Healthcheck; hc != nil && len(hc.Test) > 0 {
		app.Health = dockerHealth(hc.Test, hc.Interval, hc.Timeout, hc.Retries)
		if app.Health == nil {
			app.Notes = append(app.Notes, Note{Reason: "command-based health check is not supported", Manual: "add an HTTP health check after import"})
		}
	}
	return app
}

func dockerBuiltLocally(image string) bool {
	repo, tag := splitImageRef(image)
	if strings.Contains(repo, "/") {
		return false
	}
	return builtImageRe.MatchString(repo) || commitTagRe.MatchString(tag)
}

func dockerHealth(test []string, interval, timeout int64, retries int) *HealthCheck {
	m := healthURLRe.FindStringSubmatch(strings.Join(test, " "))
	if m == nil {
		return nil
	}
	path := m[1]
	if path == "" {
		path = "/"
	}
	return &HealthCheck{Path: path, IntervalSeconds: int(interval / 1e9), TimeoutSeconds: int(timeout / 1e9), Retries: retries}
}
