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
	Keydbs       []coolifyDatabase `json:"keydbs"`
	Dragonflies  []coolifyDatabase `json:"dragonflies"`
	Clickhouses  []coolifyDatabase `json:"clickhouses"`
}

type coolifyServer struct {
	Name        string `json:"name"`
	IP          string `json:"ip"`
	IsReachable bool   `json:"is_reachable"`
	Proxy       struct {
		Type string `json:"type"`
	} `json:"proxy"`
	Settings struct {
		IsBuildServer bool `json:"is_build_server"`
	} `json:"settings"`
}

type coolifyDatabaseDetail struct {
	UUID          string `json:"uuid"`
	IsPublic      bool   `json:"is_public"`
	PublicPort    int    `json:"public_port"`
	PostgresUser  string `json:"postgres_user"`
	PostgresDB    string `json:"postgres_db"`
	MySQLUser     string `json:"mysql_user"`
	MySQLDatabase string `json:"mysql_database"`
	MariaUser     string `json:"mariadb_user"`
	MariaDatabase string `json:"mariadb_database"`
	MongoUser     string `json:"mongo_initdb_root_username"`
	MongoDatabase string `json:"mongo_initdb_database"`
}

func (d coolifyDatabaseDetail) user() string {
	for _, v := range []string{d.PostgresUser, d.MySQLUser, d.MariaUser, d.MongoUser} {
		if v != "" {
			return v
		}
	}
	return ""
}

func (d coolifyDatabaseDetail) database() string {
	for _, v := range []string{d.PostgresDB, d.MySQLDatabase, d.MariaDatabase, d.MongoDatabase} {
		if v != "" {
			return v
		}
	}
	return ""
}

type coolifyBackup struct {
	Frequency string `json:"frequency"`
	Enabled   bool   `json:"enabled"`
	SaveS3    bool   `json:"save_s3"`
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
	InstallCommand      string `json:"install_command"`
	BuildCommand        string `json:"build_command"`
	StartCommand        string `json:"start_command"`
	PublishDirectory    string `json:"publish_directory"`
	DockerRunOptions    string `json:"custom_docker_run_options"`
	CustomLabels        string `json:"custom_labels"`
	Redirect            string `json:"redirect"`
	InlineDockerfile    string `json:"dockerfile"`
	BasicAuthEnabled    bool   `json:"is_http_basic_auth_enabled"`
	PreviewsEnabled     bool   `json:"is_preview_deployments_enabled"`
	WebhookGithub       string `json:"manual_webhook_secret_github"`
	WebhookGitlab       string `json:"manual_webhook_secret_gitlab"`
	WebhookBitbucket    string `json:"manual_webhook_secret_bitbucket"`
	WebhookGitea        string `json:"manual_webhook_secret_gitea"`
	PrivateKeyID        int    `json:"private_key_id"`
	SourceID            int    `json:"source_id"`
	ServerName          string `json:"server_name"`
	Destination         struct {
		Server struct {
			Name string `json:"name"`
		} `json:"server"`
	} `json:"destination"`
}

func (a coolifyApp) server() string {
	if a.Destination.Server.Name != "" {
		return a.Destination.Server.Name
	}
	return a.ServerName
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
	ServerName  string `json:"server_name"`
	Destination struct {
		Server struct {
			Name string `json:"name"`
		} `json:"server"`
	} `json:"destination"`
}

type coolifyEnvVar struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	RealValue   string `json:"real_value"`
	IsPreview   bool   `json:"is_preview"`
	IsShownOnce bool   `json:"is_shown_once"`
	IsBuildTime bool   `json:"is_build_time"`
	IsBuildtime bool   `json:"is_buildtime"`
}

type coolifyStorages struct {
	Persistent []struct {
		Name      string `json:"name"`
		MountPath string `json:"mount_path"`
		HostPath  string `json:"host_path"`
		SizeBytes int64  `json:"size_bytes"`
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
	c.readServers(ctx, d)
	details := c.readDatabaseDetails(ctx, d)
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
			c.collect(ctx, d, p.Name, e.Name, res, details)
		}
	}
	return d, nil
}

func (c *CoolifySource) readServers(ctx context.Context, d *Discovery) {
	var servers []coolifyServer
	if err := c.r.get(ctx, "/api/v1/servers", nil, &servers); err != nil {
		d.Warnings = append(d.Warnings, "could not read the server list ("+err.Error()+"): server placement and proxy details are missing from this report")
		return
	}
	for _, s := range servers {
		d.Servers = append(d.Servers, Server{Name: s.Name, Address: s.IP, Proxy: s.Proxy.Type, BuildServer: s.Settings.IsBuildServer, Reachable: s.IsReachable})
	}
}

func (c *CoolifySource) readDatabaseDetails(ctx context.Context, d *Discovery) map[string]coolifyDatabaseDetail {
	var list []coolifyDatabaseDetail
	if err := c.r.get(ctx, "/api/v1/databases", nil, &list); err != nil {
		d.Warnings = append(d.Warnings, "could not read database details ("+err.Error()+"): public ports, database names and backup schedules are missing from this report")
		return nil
	}
	out := make(map[string]coolifyDatabaseDetail, len(list))
	for _, x := range list {
		out[x.UUID] = x
	}
	return out
}

func (c *CoolifySource) readBackups(ctx context.Context, uuid string) []BackupSchedule {
	var backups []coolifyBackup
	if err := c.r.get(ctx, "/api/v1/databases/"+url.PathEscape(uuid)+"/backups", nil, &backups); err != nil {
		return nil
	}
	out := make([]BackupSchedule, 0, len(backups))
	for _, b := range backups {
		out = append(out, BackupSchedule{Schedule: NormalizeSchedule(b.Frequency), Enabled: b.Enabled, Remote: b.SaveS3})
	}
	return out
}

func (c *CoolifySource) collect(ctx context.Context, d *Discovery, project, environment string, res coolifyEnvResources, details map[string]coolifyDatabaseDetail) {
	for _, a := range res.Applications {
		app, unsup := c.mapApp(ctx, project, a)
		app.Environment = environment
		if unsup != nil {
			unsup.Environment = environment
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
			row := Database{SourceID: db.UUID, Name: db.Name, Project: project, Engine: g.engine, Version: ver,
				Environment: environment, InternalHost: db.UUID}
			if det, ok := details[db.UUID]; ok {
				row.User, row.DBName, row.Public, row.PublicPort = det.user(), det.database(), det.IsPublic, det.PublicPort
			}
			row.Backups = c.readBackups(ctx, db.UUID)
			d.Databases = append(d.Databases, row)
		}
	}
	others := []struct {
		engine string
		list   []coolifyDatabase
	}{{"KeyDB", res.Keydbs}, {"Dragonfly", res.Dragonflies}, {"ClickHouse", res.Clickhouses}}
	for _, g := range others {
		for _, db := range g.list {
			d.Unsupported = append(d.Unsupported, Unsupported{
				Kind: "database", SourceID: db.UUID, Name: db.Name, Project: project, Environment: environment, Source: g.engine + " database",
				Reason: g.engine + " is not a managed database engine here, so it is not created or copied",
				Manual: "run it as a container or catalog template, then move the data with its own dump tool (" + otherEngineTool(g.engine) + ")",
			})
		}
	}
	for _, s := range res.Services {
		server := s.Destination.Server.Name
		if server == "" {
			server = s.ServerName
		}
		d.Unsupported = append(d.Unsupported, Unsupported{
			Kind: "service", SourceID: s.UUID, Name: s.Name, Project: project, Environment: environment, Server: server, Source: "service " + s.ServiceType,
			Reason: "one-click service stacks are Docker Compose projects (" + s.ServiceType + ")",
			Manual: "export the service's compose file from the source and deploy it with the compose deploy command",
		})
	}
}

func otherEngineTool(engine string) string {
	switch engine {
	case "ClickHouse":
		return "clickhouse-backup or INSERT INTO ... SELECT FROM remote()"
	default:
		return "a Redis RDB snapshot or redis-cli --rdb"
	}
}

func (c *CoolifySource) mapApp(ctx context.Context, project string, a coolifyApp) (App, *Unsupported) {
	app := App{SourceID: a.UUID, Name: a.Name, Project: project, Kind: SourceUnknown, Replicas: 1,
		Domains: splitHosts(a.FQDN), Port: firstPort(a.PortsExposes),
		MemoryBytes: parseMemory(a.LimitsMemory), NanoCPUs: parseCPUs(a.LimitsCPUs),
		Server: a.server(), BuildPack: a.BuildPack, PrivateRepo: a.PrivateKeyID != 0 || a.SourceID != 0}
	switch strings.ToLower(a.BuildPack) {
	case "dockercompose":
		return App{}, &Unsupported{Kind: "app", SourceID: a.UUID, Name: a.Name, Project: project, Server: a.server(),
			Source: "docker compose", Domains: app.Domains, Port: app.Port,
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
	app.Notes = append(app.Notes, sourceSettingNotes(a)...)
	c.readDetails(ctx, &app)
	return app, nil
}

// sourceSettingNotes lists settings that have no field here, each with the value so nothing is lost silently.
func sourceSettingNotes(a coolifyApp) []Note {
	var notes []Note
	add := func(area, reason, manual string) {
		notes = append(notes, Note{Area: area, Reason: reason, Manual: manual})
	}
	if a.PreDeployment != "" || a.PostDeployment != "" {
		add("command", "pre or post deployment commands are not imported", "add them as deploy hooks in the app settings")
	}
	for _, c := range []struct{ name, val string }{{"install", a.InstallCommand}, {"build", a.BuildCommand}, {"start", a.StartCommand}} {
		if strings.TrimSpace(c.val) != "" {
			add("command", "custom "+c.name+" command is not imported: "+c.val, "Railpack detects "+c.name+" steps itself; set the matching override in the app settings or the Dockerfile if its result differs")
		}
	}
	if a.PublishDirectory != "" && strings.EqualFold(a.BuildPack, "static") {
		add("command", "publish directory "+a.PublishDirectory+" is not imported", "set the static output directory in the app settings")
	}
	if a.DockerRunOptions != "" {
		add("runtime", "custom docker run options are not imported: "+a.DockerRunOptions, "translate each option (capabilities, devices, ulimits, shm size) into app settings by hand")
	}
	if strings.TrimSpace(a.CustomLabels) != "" {
		add("runtime", "custom container labels (often Traefik middleware) are not imported", "recreate redirects, headers or rate limits as ingress settings")
	}
	if a.BasicAuthEnabled {
		add("access", "HTTP basic auth was enabled on the source and is not imported, the staged app would be public", "enable basic auth on the app before routing a domain")
	}
	if r := strings.ToLower(strings.TrimSpace(a.Redirect)); r != "" && r != "both" {
		add("domain", "www redirect rule "+r+" is not imported", "add the redirect in the app's domain settings")
	}
	if a.WebhookGithub != "" || a.WebhookGitlab != "" || a.WebhookBitbucket != "" || a.WebhookGitea != "" {
		add("webhook", "a git webhook secret is set, so the git host calls the source to auto deploy", "add a webhook for this app on the git host pointing here, then remove the old one")
	}
	if a.PreviewsEnabled {
		add("preview", "preview deployments were enabled and are not imported", "enable preview environments on the app after cutover")
	}
	if strings.TrimSpace(a.InlineDockerfile) != "" && a.GitRepository == "" {
		add("source", "the app builds from a Dockerfile stored in the source database with no git repository", "commit the Dockerfile to a repository, or build and push an image, then set it as the app source")
	}
	return notes
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
		app.Env = append(app.Env, Env{Key: e.Key, Value: val, Secret: e.IsShownOnce || LooksSecret(e.Key), BuildTime: e.IsBuildTime || e.IsBuildtime})
	}
	var buildTime []string
	for _, e := range app.Env {
		if e.BuildTime {
			buildTime = append(buildTime, e.Key)
		}
	}
	if len(buildTime) > 0 {
		app.Notes = append(app.Notes, Note{Area: "env", Reason: "build-time only variables are imported as runtime variables: " + strings.Join(buildTime, ", "),
			Manual: "if a variable must not reach the running container, remove it and pass it as a build argument instead"})
	}
	if redacted {
		app.Notes = append(app.Notes, Note{Reason: "some variables came back empty; the token may lack the read:sensitive ability", Manual: "re-run with a token that has read:sensitive or set those values by hand"})
	}
	var st coolifyStorages
	if err := c.r.get(ctx, base+"/storages", nil, &st); err == nil {
		for _, p := range st.Persistent {
			app.Volumes = append(app.Volumes, Volume{Name: p.Name, HostPath: p.HostPath, ContainerPath: p.MountPath, SizeBytes: p.SizeBytes})
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
