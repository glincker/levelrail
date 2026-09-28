package platformimport

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// CoolifySource reads a Coolify instance through its bearer-token API v1.
type CoolifySource struct {
	r *requester
}

// NewCoolify builds a Coolify source. The token is kept in memory only.
func NewCoolify(baseURL, token string, o ClientOptions) (*CoolifySource, error) {
	r, err := newRequester(baseURL, o, token)
	if err != nil {
		return nil, err
	}
	r.headers["Authorization"] = "Bearer " + token
	return &CoolifySource{r: r}, nil
}

type coolifyProject struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

type coolifyEnvironment struct {
	Name string `json:"name"`
	UUID string `json:"uuid"`
}

type coolifyEnvResources struct {
	Applications []coolifyApp      `json:"applications"`
	Postgresqls  []coolifyDatabase `json:"postgresqls"`
	Redis        []coolifyDatabase `json:"redis"`
	Mongodbs     []coolifyDatabase `json:"mongodbs"`
	Mysqls       []coolifyDatabase `json:"mysqls"`
	Mariadbs     []coolifyDatabase `json:"mariadbs"`
	Services     []coolifyService  `json:"services"`
}

type coolifyApp struct {
	UUID                string `json:"uuid"`
	Name                string `json:"name"`
	FQDN                string `json:"fqdn"`
	BuildPack           string `json:"build_pack"`
	GitRepository       string `json:"git_repository"`
	GitBranch           string `json:"git_branch"`
	DockerImage         string `json:"docker_registry_image_name"`
	DockerTag           string `json:"docker_registry_image_tag"`
	DockerfileLocation  string `json:"dockerfile_location"`
	BaseDirectory       string `json:"base_directory"`
	PortsExposes        string `json:"ports_exposes"`
	HealthCheckEnabled  bool   `json:"health_check_enabled"`
	HealthCheckPath     string `json:"health_check_path"`
	HealthCheckType     string `json:"health_check_type"`
	HealthCheckInterval int    `json:"health_check_interval"`
	HealthCheckTimeout  int    `json:"health_check_timeout"`
	HealthCheckRetries  int    `json:"health_check_retries"`
	LimitsMemory        string `json:"limits_memory"`
	LimitsCPUs          string `json:"limits_cpus"`
	PreDeployment       string `json:"pre_deployment_command"`
	PostDeployment      string `json:"post_deployment_command"`
}

type coolifyDatabase struct {
	UUID  string `json:"uuid"`
	Name  string `json:"name"`
	Image string `json:"image"`
}

type coolifyService struct {
	UUID        string `json:"uuid"`
	Name        string `json:"name"`
	ServiceType string `json:"service_type"`
}

type coolifyEnvVar struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	RealValue   string `json:"real_value"`
	IsPreview   bool   `json:"is_preview"`
	IsShownOnce bool   `json:"is_shown_once"`
}

type coolifyStorages struct {
	Persistent []struct {
		Name      string `json:"name"`
		MountPath string `json:"mount_path"`
		HostPath  string `json:"host_path"`
	} `json:"persistent_storages"`
	Files []struct {
		FSPath    string `json:"fs_path"`
		MountPath string `json:"mount_path"`
	} `json:"file_storages"`
}

type coolifyTask struct {
	Name      string `json:"name"`
	Command   string `json:"command"`
	Frequency string `json:"frequency"`
	Enabled   bool   `json:"enabled"`
}

// Discover walks projects, environments, applications, databases and services.
func (c *CoolifySource) Discover(ctx context.Context) (*Discovery, error) {
	var projects []coolifyProject
	if err := c.r.get(ctx, "/api/v1/projects", nil, &projects); err != nil {
		return nil, fmt.Errorf("list coolify projects: %w", err)
	}
	if len(projects) > MaxPages {
		projects = projects[:MaxPages]
	}
	d := &Discovery{Platform: Coolify}
	for _, p := range projects {
		d.Projects = append(d.Projects, p.Name)
		var envs []coolifyEnvironment
		if err := c.r.get(ctx, "/api/v1/projects/"+url.PathEscape(p.UUID)+"/environments", nil, &envs); err != nil {
			return nil, fmt.Errorf("list environments of project %q: %w", p.Name, err)
		}
		for _, e := range envs {
			var res coolifyEnvResources
			if err := c.r.get(ctx, "/api/v1/projects/"+url.PathEscape(p.UUID)+"/"+url.PathEscape(e.Name), nil, &res); err != nil {
				return nil, fmt.Errorf("read environment %q of project %q: %w", e.Name, p.Name, err)
			}
			c.collect(ctx, d, p.Name, res)
		}
	}
	return d, nil
}

func (c *CoolifySource) collect(ctx context.Context, d *Discovery, project string, res coolifyEnvResources) {
	for _, a := range res.Applications {
		app, unsup := c.mapApp(ctx, project, a)
		if unsup != nil {
			d.Unsupported = append(d.Unsupported, *unsup)
			continue
		}
		d.Apps = append(d.Apps, app)
	}
	dbs := []struct {
		engine string
		list   []coolifyDatabase
	}{{"postgres", res.Postgresqls}, {"redis", res.Redis}, {"mongodb", res.Mongodbs}, {"mysql", res.Mysqls}, {"mariadb", res.Mariadbs}}
	for _, g := range dbs {
		for _, db := range g.list {
			_, ver := engineFromImage(db.Image)
			d.Databases = append(d.Databases, Database{SourceID: db.UUID, Name: db.Name, Project: project, Engine: g.engine, Version: ver})
		}
	}
	for _, s := range res.Services {
		d.Unsupported = append(d.Unsupported, Unsupported{
			Kind: "service", SourceID: s.UUID, Name: s.Name,
			Reason: "one-click service stacks are Docker Compose projects (" + s.ServiceType + ")",
			Manual: "export the service's compose file from the source and deploy it with the compose deploy command",
		})
	}
}

func (c *CoolifySource) mapApp(ctx context.Context, project string, a coolifyApp) (App, *Unsupported) {
	app := App{SourceID: a.UUID, Name: a.Name, Project: project, Kind: SourceUnknown, Replicas: 1,
		Domains: splitHosts(a.FQDN), Port: firstPort(a.PortsExposes),
		MemoryBytes: parseMemory(a.LimitsMemory), NanoCPUs: parseCPUs(a.LimitsCPUs)}
	switch strings.ToLower(a.BuildPack) {
	case "dockercompose":
		return App{}, &Unsupported{Kind: "app", SourceID: a.UUID, Name: a.Name,
			Reason: "application uses the Docker Compose build pack (multi-service)",
			Manual: "deploy its compose file with the compose deploy command"}
	case "dockerimage":
		app.Kind = SourceImage
		app.Image = a.DockerImage
		if a.DockerTag != "" && !strings.Contains(a.DockerImage, ":") {
			app.Image += ":" + a.DockerTag
		}
	case "dockerfile":
		app.Kind, app.BuildMethod, app.BuildPath = SourceGit, "dockerfile", a.DockerfileLocation
	case "nixpacks":
		app.Kind, app.BuildMethod = SourceGit, "railpack"
		app.Notes = append(app.Notes, Note{Reason: "source used Nixpacks; imported as Railpack auto-detect, which may build differently", Manual: "run a build and compare the result before switching traffic"})
	case "static":
		app.Kind, app.BuildMethod = SourceGit, "static"
	default:
		app.Notes = append(app.Notes, Note{Reason: "unknown build pack " + a.BuildPack, Manual: "set the build method after import"})
	}
	if app.Kind == SourceGit {
		app.GitURL = gitURLFromRepo(a.GitRepository, "github.com")
		app.GitBranch = a.GitBranch
		app.BuildPath = strings.Trim(a.BaseDirectory, "/")
		if app.BuildMethod == "dockerfile" {
			app.BuildPath = strings.Trim(strings.Trim(a.BaseDirectory, "/")+"/"+strings.Trim(a.DockerfileLocation, "/"), "/")
		}
	}
	httpCheck := a.HealthCheckType == "" || strings.EqualFold(a.HealthCheckType, "http")
	if a.HealthCheckEnabled && httpCheck {
		app.Health = &HealthCheck{Path: a.HealthCheckPath, IntervalSeconds: a.HealthCheckInterval, TimeoutSeconds: a.HealthCheckTimeout, Retries: a.HealthCheckRetries}
	} else if a.HealthCheckEnabled {
		app.Notes = append(app.Notes, Note{Reason: "command-based health check is not supported", Manual: "add an HTTP health check after import"})
	}
	if a.PreDeployment != "" || a.PostDeployment != "" {
		app.Notes = append(app.Notes, Note{Reason: "pre or post deployment commands are not imported", Manual: "add them as deploy hooks in the app settings"})
	}
	c.readDetails(ctx, &app)
	return app, nil
}

func (c *CoolifySource) readDetails(ctx context.Context, app *App) {
	base := "/api/v1/applications/" + url.PathEscape(app.SourceID)
	var envs []coolifyEnvVar
	if err := c.r.get(ctx, base+"/envs", nil, &envs); err != nil {
		app.Notes = append(app.Notes, Note{Reason: "could not read environment variables: " + err.Error(), Manual: "set them after import"})
	}
	redacted := false
	seen := map[string]bool{}
	for _, e := range envs {
		if e.IsPreview || seen[e.Key] {
			continue
		}
		seen[e.Key] = true
		val := e.RealValue
		if val == "" {
			val = e.Value
		}
		if val == "" {
			redacted = true
		}
		app.Env = append(app.Env, Env{Key: e.Key, Value: val, Secret: e.IsShownOnce || LooksSecret(e.Key)})
	}
	if redacted {
		app.Notes = append(app.Notes, Note{Reason: "some variables came back empty; the token may lack the read:sensitive ability", Manual: "re-run with a token that has read:sensitive or set those values by hand"})
	}
	var st coolifyStorages
	if err := c.r.get(ctx, base+"/storages", nil, &st); err == nil {
		for _, p := range st.Persistent {
			app.Volumes = append(app.Volumes, Volume{Name: p.Name, HostPath: p.HostPath, ContainerPath: p.MountPath})
		}
		for _, f := range st.Files {
			app.Notes = append(app.Notes, Note{Reason: "file mount " + f.MountPath + " is not imported", Manual: "recreate the file content inside the image or as a volume"})
		}
	}
	var tasks []coolifyTask
	if err := c.r.get(ctx, base+"/scheduled-tasks", nil, &tasks); err == nil {
		for _, t := range tasks {
			if t.Enabled {
				app.Crons = append(app.Crons, Cron{Name: t.Name, Schedule: t.Frequency, Command: t.Command})
			}
		}
	}
}
