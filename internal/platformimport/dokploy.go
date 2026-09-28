package platformimport

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// DokploySource reads a Dokploy instance through its x-api-key API.
type DokploySource struct {
	r *requester
}

// NewDokploy builds a Dokploy source. The API key is kept in memory only.
func NewDokploy(baseURL, apiKey string, o ClientOptions) (*DokploySource, error) {
	r, err := newRequester(baseURL, o, apiKey)
	if err != nil {
		return nil, err
	}
	r.headers["x-api-key"] = apiKey
	return &DokploySource{r: r}, nil
}

type dokployResources struct {
	Applications []dokployAppRef `json:"applications"`
	Compose      []dokployRef    `json:"compose"`
	Postgres     []dokployDB     `json:"postgres"`
	Mysql        []dokployDB     `json:"mysql"`
	Mariadb      []dokployDB     `json:"mariadb"`
	Mongo        []dokployDB     `json:"mongo"`
	Redis        []dokployDB     `json:"redis"`
}

type dokployProject struct {
	ProjectID string `json:"projectId"`
	Name      string `json:"name"`
	dokployResources
	Environments []struct {
		dokployResources
	} `json:"environments"`
}

type dokployRef struct {
	ID   string `json:"composeId"`
	Name string `json:"name"`
}

type dokployAppRef struct {
	ApplicationID string `json:"applicationId"`
	Name          string `json:"name"`
}

type dokployDB struct {
	ID          string `json:"postgresId"`
	MysqlID     string `json:"mysqlId"`
	MariadbID   string `json:"mariadbId"`
	MongoID     string `json:"mongoId"`
	RedisID     string `json:"redisId"`
	Name        string `json:"name"`
	DockerImage string `json:"dockerImage"`
}

func (d dokployDB) id() string {
	for _, v := range []string{d.ID, d.MysqlID, d.MariadbID, d.MongoID, d.RedisID} {
		if v != "" {
			return v
		}
	}
	return d.Name
}

type dokployDomain struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type dokployMount struct {
	Type       string `json:"type"`
	HostPath   string `json:"hostPath"`
	VolumeName string `json:"volumeName"`
	MountPath  string `json:"mountPath"`
}

type dokployApp struct {
	ApplicationID       string          `json:"applicationId"`
	Name                string          `json:"name"`
	SourceType          string          `json:"sourceType"`
	BuildType           string          `json:"buildType"`
	BuildPath           string          `json:"buildPath"`
	Dockerfile          string          `json:"dockerfile"`
	DockerImage         string          `json:"dockerImage"`
	Env                 string          `json:"env"`
	Replicas            int             `json:"replicas"`
	MemoryLimit         string          `json:"memoryLimit"`
	CPULimit            string          `json:"cpuLimit"`
	Owner               string          `json:"owner"`
	Repository          string          `json:"repository"`
	Branch              string          `json:"branch"`
	GitlabOwner         string          `json:"gitlabOwner"`
	GitlabRepository    string          `json:"gitlabRepository"`
	GitlabBranch        string          `json:"gitlabBranch"`
	BitbucketOwner      string          `json:"bitbucketOwner"`
	BitbucketRepository string          `json:"bitbucketRepository"`
	BitbucketBranch     string          `json:"bitbucketBranch"`
	GiteaOwner          string          `json:"giteaOwner"`
	GiteaRepository     string          `json:"giteaRepository"`
	GiteaBranch         string          `json:"giteaBranch"`
	CustomGitURL        string          `json:"customGitUrl"`
	CustomGitBranch     string          `json:"customGitBranch"`
	Domains             []dokployDomain `json:"domains"`
	Mounts              []dokployMount  `json:"mounts"`
	HealthCheckSwarm    *struct {
		Test     []string `json:"Test"`
		Interval int64    `json:"Interval"`
		Timeout  int64    `json:"Timeout"`
		Retries  int      `json:"Retries"`
	} `json:"healthCheckSwarm"`
}

// Discover lists projects (which embed their resources) and reads each app.
func (s *DokploySource) Discover(ctx context.Context) (*Discovery, error) {
	var projects []dokployProject
	if err := s.r.get(ctx, "/api/project.all", nil, &projects); err != nil {
		return nil, fmt.Errorf("list dokploy projects: %w", err)
	}
	if len(projects) > MaxPages {
		projects = projects[:MaxPages]
	}
	d := &Discovery{Platform: Dokploy}
	for _, p := range projects {
		d.Projects = append(d.Projects, p.Name)
		sets := []dokployResources{p.dokployResources}
		for _, e := range p.Environments {
			sets = append(sets, e.dokployResources)
		}
		for _, res := range sets {
			s.collect(ctx, d, p.Name, res)
		}
	}
	return d, nil
}

func (s *DokploySource) collect(ctx context.Context, d *Discovery, project string, res dokployResources) {
	for _, ref := range res.Applications {
		var full dokployApp
		if err := s.r.get(ctx, "/api/application.one", url.Values{"applicationId": {ref.ApplicationID}}, &full); err != nil {
			d.Unsupported = append(d.Unsupported, Unsupported{Kind: "app", SourceID: ref.ApplicationID, Name: ref.Name,
				Reason: "could not read application: " + err.Error(), Manual: "re-run the discovery or import it by hand"})
			continue
		}
		app, unsup := mapDokployApp(project, full)
		if unsup != nil {
			d.Unsupported = append(d.Unsupported, *unsup)
			continue
		}
		d.Apps = append(d.Apps, app)
	}
	for _, c := range res.Compose {
		d.Unsupported = append(d.Unsupported, Unsupported{Kind: "compose", SourceID: c.ID, Name: c.Name,
			Reason: "compose stacks are multi-service", Manual: "deploy the compose file with the compose deploy command"})
	}
	dbs := []struct {
		engine string
		list   []dokployDB
	}{{"postgres", res.Postgres}, {"mysql", res.Mysql}, {"mariadb", res.Mariadb}, {"mongodb", res.Mongo}, {"redis", res.Redis}}
	for _, g := range dbs {
		for _, db := range g.list {
			_, ver := engineFromImage(db.DockerImage)
			d.Databases = append(d.Databases, Database{SourceID: db.id(), Name: db.Name, Project: project, Engine: g.engine, Version: ver})
		}
	}
}

func mapDokployApp(project string, a dokployApp) (App, *Unsupported) {
	app := App{SourceID: a.ApplicationID, Name: a.Name, Project: project, Kind: SourceUnknown, Replicas: a.Replicas,
		Env: parseEnvBlob(a.Env)}
	if app.Replicas < 1 {
		app.Replicas = 1
	}
	if n, err := strconv.ParseInt(strings.TrimSpace(a.MemoryLimit), 10, 64); err == nil && n > 0 {
		app.MemoryBytes = n
	}
	if n, err := strconv.ParseInt(strings.TrimSpace(a.CPULimit), 10, 64); err == nil && n > 0 {
		app.NanoCPUs = n
	}
	for _, dm := range a.Domains {
		if dm.Host != "" {
			app.Domains = append(app.Domains, strings.ToLower(dm.Host))
			if app.Port == 0 {
				app.Port = dm.Port
			}
		}
	}
	for _, m := range a.Mounts {
		switch m.Type {
		case "volume":
			app.Volumes = append(app.Volumes, Volume{Name: m.VolumeName, ContainerPath: m.MountPath})
		case "bind":
			app.Volumes = append(app.Volumes, Volume{HostPath: m.HostPath, ContainerPath: m.MountPath})
		default:
			app.Notes = append(app.Notes, Note{Reason: "file mount " + m.MountPath + " is not imported", Manual: "bake the file into the image or use a volume"})
		}
	}
	if hc := a.HealthCheckSwarm; hc != nil && len(hc.Test) > 0 {
		if path := healthPathFromTest(hc.Test); path != "" {
			app.Health = &HealthCheck{Path: path, IntervalSeconds: int(hc.Interval / 1e9), TimeoutSeconds: int(hc.Timeout / 1e9), Retries: hc.Retries}
		} else {
			app.Notes = append(app.Notes, Note{Reason: "swarm health check command could not be converted to an HTTP probe", Manual: "add an HTTP health check after import"})
		}
	}
	switch strings.ToLower(a.SourceType) {
	case "docker":
		app.Kind, app.Image = SourceImage, a.DockerImage
	case "drop":
		return App{}, &Unsupported{Kind: "app", SourceID: a.ApplicationID, Name: a.Name,
			Reason: "source is an uploaded archive, there is nothing to pull", Manual: "push the code to a git repository and create the app from it"}
	case "github":
		app.Kind, app.GitURL, app.GitBranch = SourceGit, "https://github.com/"+a.Owner+"/"+a.Repository, a.Branch
	case "bitbucket":
		app.Kind, app.GitURL, app.GitBranch = SourceGit, "https://bitbucket.org/"+a.BitbucketOwner+"/"+a.BitbucketRepository, a.BitbucketBranch
	case "gitlab":
		app.Kind, app.GitBranch = SourceGit, a.GitlabBranch
		app.Notes = append(app.Notes, Note{Reason: "GitLab instance host is not exposed by the API", Manual: "set the repository URL for " + a.GitlabOwner + "/" + a.GitlabRepository})
	case "gitea":
		app.Kind, app.GitBranch = SourceGit, a.GiteaBranch
		app.Notes = append(app.Notes, Note{Reason: "Gitea instance host is not exposed by the API", Manual: "set the repository URL for " + a.GiteaOwner + "/" + a.GiteaRepository})
	case "git":
		app.Kind, app.GitURL, app.GitBranch = SourceGit, a.CustomGitURL, a.CustomGitBranch
	}
	if app.Kind == SourceGit {
		switch strings.ToLower(a.BuildType) {
		case "dockerfile":
			app.BuildMethod = "dockerfile"
			app.BuildPath = strings.Trim(strings.Trim(a.BuildPath, "/")+"/"+strings.Trim(a.Dockerfile, "/"), "/")
		case "nixpacks", "railpack", "heroku_buildpacks", "paketo_buildpacks":
			app.BuildMethod = "railpack"
			app.Notes = append(app.Notes, Note{Reason: "source build type " + a.BuildType + " is imported as Railpack auto-detect", Manual: "run a build and compare the result before switching traffic"})
		case "static":
			app.BuildMethod = "static"
		}
	}
	return app, nil
}

func healthPathFromTest(test []string) string {
	for _, arg := range test {
		if strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://") {
			if u, err := url.Parse(arg); err == nil {
				p := u.Path
				if p == "" {
					p = "/"
				}
				return p
			}
		}
	}
	return ""
}

// parseEnvBlob parses "KEY=VALUE" lines, skipping blanks and comments.
func parseEnvBlob(blob string) []Env {
	var out []Env
	for _, line := range strings.Split(blob, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
		out = append(out, Env{Key: k, Value: v, Secret: LooksSecret(k)})
	}
	return out
}
