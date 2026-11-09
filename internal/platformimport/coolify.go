package platformimport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/spec"
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
	// S3StorageUUID names the destination Coolify uploads to; its keys are never read.
	S3StorageUUID string `json:"s3_storage_uuid"`
	S3Bucket      string `json:"s3_bucket"`
}

type coolifyApp struct {
	UUID               string `json:"uuid"`
	Name               string `json:"name"`
	FQDN               string `json:"fqdn"`
	BuildPack          string `json:"build_pack"`
	GitRepository      string `json:"git_repository"`
	GitBranch          string `json:"git_branch"`
	DockerImage        string `json:"docker_registry_image_name"`
	DockerTag          string `json:"docker_registry_image_tag"`
	DockerfileLocation string `json:"dockerfile_location"`
	BaseDirectory      string `json:"base_directory"`
	PortsExposes       string `json:"ports_exposes"`
	HealthCheckEnabled bool   `json:"health_check_enabled"`
	HealthCheckPath    string `json:"health_check_path"`
	HealthCheckType    string `json:"health_check_type"`
	// Field name is a best guess, unverified against a real instance.
	HealthCheckCommand  string             `json:"health_check_command"`
	HealthCheckInterval int                `json:"health_check_interval"`
	HealthCheckTimeout  int                `json:"health_check_timeout"`
	HealthCheckRetries  int                `json:"health_check_retries"`
	LimitsMemory        string             `json:"limits_memory"`
	LimitsCPUs          string             `json:"limits_cpus"`
	LimitsSwap          string             `json:"limits_memory_swap"`
	LimitsSwappiness    int                `json:"limits_memory_swappiness"`
	LimitsReservation   string             `json:"limits_memory_reservation"`
	PreDeployment       string             `json:"pre_deployment_command"`
	PostDeployment      string             `json:"post_deployment_command"`
	InstallCommand      string             `json:"install_command"`
	BuildCommand        string             `json:"build_command"`
	StartCommand        string             `json:"start_command"`
	PublishDirectory    string             `json:"publish_directory"`
	DockerRunOptions    string             `json:"custom_docker_run_options"`
	CustomLabels        string             `json:"custom_labels"`
	Redirect            string             `json:"redirect"`
	InlineDockerfile    string             `json:"dockerfile"`
	DockerComposeRaw    string             `json:"docker_compose_raw"`
	BasicAuthEnabled    bool               `json:"is_http_basic_auth_enabled"`
	PreviewsEnabled     bool               `json:"is_preview_deployments_enabled"`
	WebhookGithub       string             `json:"manual_webhook_secret_github"`
	WebhookGitlab       string             `json:"manual_webhook_secret_gitlab"`
	WebhookBitbucket    string             `json:"manual_webhook_secret_bitbucket"`
	WebhookGitea        string             `json:"manual_webhook_secret_gitea"`
	PrivateKeyID        int                `json:"private_key_id"`
	SourceID            int                `json:"source_id"`
	ServerName          string             `json:"server_name"`
	Destination         coolifyDestination `json:"destination"`
	// Tags arrive as names or as objects, so they are parsed leniently.
	Tags json.RawMessage `json:"tags"`
}

// coolifyDestination is the docker destination a resource runs on; its network is read from the
// resource payload already fetched, so network detection costs no extra request.
type coolifyDestination struct {
	Network string `json:"network"`
	Server  struct {
		Name string `json:"name"`
	} `json:"server"`
}

// customNetwork returns the destination network when it is not the instance default.
func (d coolifyDestination) customNetwork() string {
	n := strings.TrimSpace(d.Network)
	if n == "" || n == "coolify" {
		return ""
	}
	return n
}

func (a coolifyApp) server() string {
	if a.Destination.Server.Name != "" {
		return a.Destination.Server.Name
	}
	return a.ServerName
}

type coolifyDatabase struct {
	UUID        string             `json:"uuid"`
	Name        string             `json:"name"`
	Image       string             `json:"image"`
	Destination coolifyDestination `json:"destination"`
}

type coolifyService struct {
	UUID        string             `json:"uuid"`
	Name        string             `json:"name"`
	ServiceType string             `json:"service_type"`
	ServerName  string             `json:"server_name"`
	Destination coolifyDestination `json:"destination"`
}

// ComposeRaw is the field name assumed from the Coolify API docs, not confirmed against a live instance.
type coolifyServiceDetail struct {
	ComposeRaw string `json:"docker_compose_raw"`
	Compose    string `json:"docker_compose"`
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
		FSPath      string `json:"fs_path"`
		MountPath   string `json:"mount_path"`
		Content     string `json:"content"`
		IsDirectory bool   `json:"is_directory"`
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
	nets := &networkTally{byNet: map[string][]string{}, appIdx: map[int]string{}}
	for _, p := range projects {
		d.Projects = append(d.Projects, p.Name)
		var envs []coolifyEnvironment
		if err := c.r.get(ctx, "/api/v1/projects/"+url.PathEscape(p.UUID)+"/environments", nil, &envs); err != nil {
			return nil, fmt.Errorf("list environments of project %q: %w", p.Name, err)
		}
		for _, e := range envs {
			d.Environments = append(d.Environments, SourceEnvironment{Project: p.Name, Name: e.Name})
			var res coolifyEnvResources
			if err := c.r.get(ctx, "/api/v1/projects/"+url.PathEscape(p.UUID)+"/"+url.PathEscape(e.Name), nil, &res); err != nil {
				return nil, fmt.Errorf("read environment %q of project %q: %w", e.Name, p.Name, err)
			}
			c.collect(ctx, d, p.Name, e.Name, res, details, nets)
		}
	}
	nets.annotate(d)
	return d, nil
}

// networkTally records which imported resources sit on each custom network, so apps that
// share one can be flagged after every environment is read.
type networkTally struct {
	byNet  map[string][]string
	appIdx map[int]string
}

func (n *networkTally) add(net, label string) {
	if net != "" {
		n.byNet[net] = append(n.byNet[net], label)
	}
}

func (n *networkTally) annotate(d *Discovery) {
	for i, net := range n.appIdx {
		members := n.byNet[net]
		if len(members) < 2 {
			continue
		}
		var others []string
		for _, m := range members {
			if m != d.Apps[i].Name {
				others = append(others, m)
			}
		}
		d.Apps[i].NetworkPeers = others
		d.Apps[i].Notes = append(d.Apps[i].Notes, Note{Area: "network",
			Reason: "shares the custom network " + net + " with " + strings.Join(others, ", ") + ", other resources may reach it by container name",
			Manual: "put the related resources in one project here and use the service name or the internal host from the report as the address"})
	}
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

func (c *CoolifySource) readBackups(ctx context.Context, uuid string) []coolifyBackup {
	var backups []coolifyBackup
	if err := c.r.get(ctx, "/api/v1/databases/"+url.PathEscape(uuid)+"/backups", nil, &backups); err != nil {
		return nil
	}
	return backups
}

func backupSchedules(backups []coolifyBackup) []BackupSchedule {
	out := make([]BackupSchedule, 0, len(backups))
	for _, b := range backups {
		out = append(out, BackupSchedule{Schedule: NormalizeSchedule(b.Frequency), Enabled: b.Enabled, Remote: b.SaveS3})
	}
	return out
}

func (c *CoolifySource) collect(ctx context.Context, d *Discovery, project, environment string, res coolifyEnvResources, details map[string]coolifyDatabaseDetail, nets *networkTally) {
	for _, a := range res.Applications {
		if strings.EqualFold(a.BuildPack, "dockercompose") {
			d.ComposeApps = append(d.ComposeApps, c.composeApp(ctx, project, environment, a))
			continue
		}
		app, unsup := c.mapApp(ctx, project, a)
		app.Environment = environment
		nets.add(a.Destination.customNetwork(), a.Name)
		if unsup != nil {
			unsup.Environment = environment
			d.Unsupported = append(d.Unsupported, *unsup)
			continue
		}
		if n := a.Destination.customNetwork(); n != "" {
			nets.appIdx[len(d.Apps)] = n
		}
		d.Apps = append(d.Apps, app)
	}
	dbs := []struct {
		engine string
		list   []coolifyDatabase
	}{{"postgres", res.Postgresqls}, {"redis", res.Redis}, {"mongodb", res.Mongodbs}, {"mysql", res.Mysqls}, {"mariadb", res.Mariadbs}}
	for _, g := range dbs {
		for _, db := range g.list {
			nets.add(db.Destination.customNetwork(), db.Name)
			_, ver := engineFromImage(db.Image)
			row := Database{SourceID: db.UUID, Name: db.Name, Project: project, Engine: g.engine, Version: ver,
				Environment: environment, InternalHost: db.UUID}
			if det, ok := details[db.UUID]; ok {
				row.User, row.DBName, row.Public, row.PublicPort = det.user(), det.database(), det.IsPublic, det.PublicPort
			}
			backups := c.readBackups(ctx, db.UUID)
			row.Backups = backupSchedules(backups)
			reportBackups(d, row, backups)
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
		nets.add(s.Destination.customNetwork(), s.Name)
		server := s.Destination.Server.Name
		if server == "" {
			server = s.ServerName
		}
		d.Services = append(d.Services, c.readService(ctx, d, project, environment, server, s))
		d.Unsupported = append(d.Unsupported, Unsupported{
			Kind: "service", SourceID: s.UUID, Name: s.Name, Project: project, Environment: environment, Server: server, Source: "service " + s.ServiceType,
			Reason: "one-click service stacks are Docker Compose projects (" + s.ServiceType + ")",
			Manual: "export the service's compose file from the source and deploy it with the compose deploy command",
		})
	}
}

var composeSecretLineRe = regexp.MustCompile(`^(\s*(?:-\s*)?["']?)([A-Za-z_][A-Za-z0-9_]*)(["']?\s*[:=]\s*)(.+)$`)

// maskCompose hides inline values of secret-looking keys; ${VAR} references stay.
func maskCompose(body string) string {
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		m := composeSecretLineRe.FindStringSubmatch(l)
		if m == nil || !LooksSecret(m[2]) || strings.HasPrefix(strings.TrimSpace(m[4]), "$") {
			continue
		}
		lines[i] = m[1] + m[2] + m[3] + SecretMask
	}
	return strings.Join(lines, "\n")
}

// readService fetches one service's compose body and env, read only; failures become warnings.
func (c *CoolifySource) readService(ctx context.Context, d *Discovery, project, environment, server string, s coolifyService) Service {
	out := Service{SourceID: s.UUID, Name: s.Name, Project: project, Environment: environment, Server: server, Type: s.ServiceType}
	base := "/api/v1/services/" + url.PathEscape(s.UUID)
	var det coolifyServiceDetail
	if err := c.r.get(ctx, base, nil, &det); err != nil {
		d.Warnings = append(d.Warnings, "could not read the compose of service "+s.Name+" ("+err.Error()+")")
	} else {
		body := det.ComposeRaw
		if body == "" {
			body = det.Compose
		}
		out.Compose = maskCompose(body)
	}
	var envs []coolifyEnvVar
	if err := c.r.get(ctx, base+"/envs", nil, &envs); err != nil {
		d.Warnings = append(d.Warnings, "could not read the environment of service "+s.Name+" ("+err.Error()+")")
		return out
	}
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
		secret := e.IsShownOnce || LooksSecret(e.Key)
		if secret && val != "" {
			val = SecretMask
		}
		out.Env = append(out.Env, Env{Key: e.Key, Value: val, Secret: secret, BuildTime: e.IsBuildTime || e.IsBuildtime})
	}
	return out
}

// shellArg single-quotes a value so a printed command can be pasted as is.
func shellArg(v string) string {
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

// reportBackups lists each backup schedule and S3 destination as a manual item,
// since neither is created by the import and credentials are never read.
func reportBackups(d *Discovery, db Database, backups []coolifyBackup) {
	for _, b := range backups {
		if b.SaveS3 && b.S3StorageUUID != "" {
			reportS3Destination(d, db, b)
		}
		cron := NormalizeSchedule(b.Frequency)
		state := ""
		if !b.Enabled {
			state = " (disabled in the source)"
		}
		d.Unsupported = append(d.Unsupported, Unsupported{
			Kind: "backup-schedule", SourceID: db.SourceID, Name: db.Name + " backup " + cron, Project: db.Project, Environment: db.Environment, Source: "database backup schedule",
			Reason: "backup schedules are not imported" + state,
			Manual: "levelrail-cli backups schedule set " + shellArg(db.Name) + " --target <TARGET_ID> --cron " + shellArg(cron),
		})
	}
}

func reportS3Destination(d *Discovery, db Database, b coolifyBackup) {
	for _, u := range d.Unsupported {
		if u.Kind == "backup-destination" && u.SourceID == b.S3StorageUUID {
			return
		}
	}
	bucket := b.S3Bucket
	if bucket == "" {
		bucket = "<BUCKET>"
	}
	d.Unsupported = append(d.Unsupported, Unsupported{
		Kind: "backup-destination", SourceID: b.S3StorageUUID, Name: "S3 destination " + b.S3StorageUUID, Project: db.Project, Environment: db.Environment, Source: "S3 storage",
		Reason: "S3 destinations and their keys are not read or imported",
		Manual: "levelrail-cli backup-targets create --name " + shellArg(b.S3StorageUUID) + " --provider custom --endpoint <S3_ENDPOINT> --bucket " + shellArg(bucket) + " --access-key-id <KEY_ID> --secret-access-key <SECRET>",
	})
}

func taskNote(app App, t coolifyTask) Note {
	cmd := "levelrail-cli jobs create " + shellArg(SanitizeName(app.Name)) + " " + shellArg(SanitizeJobName(t.Name)) + " --schedule " + shellArg(NormalizeSchedule(t.Frequency)) + " -- sh -c " + shellArg(t.Command)
	return Note{Area: "task", Reason: "scheduled task " + t.Name + " is disabled in the source, so it was not imported", Manual: cmd}
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
			Source: "docker compose", Domains: app.Domains, Port: app.Port, Reason: composeBuildPack}
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
		app.scrubGitURL()
		app.GitBranch = a.GitBranch
	}
	httpCheck := a.HealthCheckType == "" || strings.EqualFold(a.HealthCheckType, "http")
	if a.HealthCheckEnabled && httpCheck {
		app.Health = &HealthCheck{Path: a.HealthCheckPath, IntervalSeconds: a.HealthCheckInterval, TimeoutSeconds: a.HealthCheckTimeout, Retries: a.HealthCheckRetries}
	} else if argv := coolifyHealthExec(a); a.HealthCheckEnabled && argv != nil {
		app.Health = &HealthCheck{Exec: argv, IntervalSeconds: a.HealthCheckInterval, TimeoutSeconds: a.HealthCheckTimeout, Retries: a.HealthCheckRetries}
	} else if a.HealthCheckEnabled {
		app.Notes = append(app.Notes, Note{Reason: "command-based health check is not supported", Manual: "add an HTTP health check after import"})
	}
	app.Notes = append(app.Notes, sourceSettingNotes(a)...)
	app.Previews = a.PreviewsEnabled
	first := ""
	if len(app.Domains) > 0 {
		first = app.Domains[0]
	}
	if ing := TranslateIngress(IngressSource{Labels: a.CustomLabels, BasicAuth: a.BasicAuthEnabled, Redirect: a.Redirect,
		App: SanitizeName(a.Name), Domain: first}, nil); ing.Translated() || len(ing.Manual) > 0 {
		app.Ingress = &ing
		app.Notes = append(app.Notes, ingressNotes(ing)...)
	}
	c.readDetails(ctx, &app)
	// Cleaned after the env read, so a command embedding any fetched value is dropped.
	values := make([]string, 0, len(app.Env))
	for _, e := range app.Env {
		values = append(values, e.Value)
	}
	build, buildNotes := coolifyBuildSettings(a, values)
	app.Notes = append(app.Notes, buildNotes...)
	if app.Kind == SourceGit {
		app.Build = build
		app.BuildPath = build.BaseDirectory
		if app.BuildMethod == "dockerfile" {
			app.BuildPath = strings.Trim(build.BaseDirectory+"/"+build.DockerfilePath, "/")
		}
	}
	for _, e := range app.Env {
		if e.BuildTime && app.Kind == SourceGit {
			app.Build.BuildTimeKeys = append(app.Build.BuildTimeKeys, e.Key)
		}
	}
	app.Notes = append(app.Notes, buildPlanNotes(a, app.Build)...)
	if n := nodePinNote(app.Env); n != nil {
		app.Notes = append(app.Notes, *n)
	}
	return app, nil
}

// sourceSettingNotes lists settings that have no field here, each with the value so nothing is lost silently.
// coolifyHealthExec returns the sh -c argv for a usable command check, else nil.
func coolifyHealthExec(a coolifyApp) []string {
	t := strings.ToLower(a.HealthCheckType)
	cmd := strings.TrimSpace(a.HealthCheckCommand)
	if (t != "cmd" && t != "command") || cmd == "" || len(cmd) > 4096 {
		return nil
	}
	return []string{"/bin/sh", "-c", cmd}
}

func sourceSettingNotes(a coolifyApp) []Note {
	var notes []Note
	add := func(area, reason, manual string) {
		notes = append(notes, Note{Area: area, Reason: reason, Manual: manual})
	}
	if a.DockerRunOptions != "" {
		add("runtime", "custom docker run options are not imported: "+a.DockerRunOptions, "translate each option (capabilities, devices, ulimits, shm size) into app settings by hand")
	}
	if a.WebhookGithub != "" || a.WebhookGitlab != "" || a.WebhookBitbucket != "" || a.WebhookGitea != "" {
		add("webhook", "planned: a deploy webhook secret is set, so the git host calls the source to auto deploy; the secret is never read into this report", "create a build hook (apps build-hooks create <app> --name deploy), point the git host or CI at its URL, then remove the old webhook")
	}
	if swapSet(a.LimitsSwap) || swapSet(a.LimitsReservation) {
		add("limits", "memory swap or reservation is set ("+swapSummary(a)+") and has no setting here, only the memory limit is imported", "none needed unless the app relies on swap, then size the memory limit for it")
	}
	if a.PreviewsEnabled {
		add("preview", "planned: preview deployments were enabled, previews are turned on with the default policy", "review the preview policy (limits, gate, database strategy) before the first pull request")
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
			if f.IsDirectory {
				app.Notes = append(app.Notes, Note{Area: "file", Reason: "directory mount " + f.MountPath + " is not imported", Manual: "recreate its files with the app files command or use a volume"})
				continue
			}
			af, err := NewAppFile(f.MountPath, []byte(f.Content))
			if err != nil {
				app.Notes = append(app.Notes, Note{Area: "file", Reason: "file mount was not imported: " + err.Error(), Manual: "recreate the file with the app files command"})
				continue
			}
			app.Files = append(app.Files, af)
			app.Notes = append(app.Notes, Note{Area: "file", Reason: "file mount " + af.Path + " is copied only by the migrate command", Manual: "run the migrate command with real values or set it with the app files command"})
		}
	}
	var tasks []coolifyTask
	if err := c.r.get(ctx, base+"/scheduled-tasks", nil, &tasks); err == nil {
		for _, t := range tasks {
			if !t.Enabled {
				app.Notes = append(app.Notes, taskNote(*app, t))
				continue
			}
			app.Crons = append(app.Crons, Cron{Name: t.Name, Schedule: t.Frequency, Command: t.Command})
		}
	}
}

// swapSet is true for a value other than empty or zero, the source defaults.
func swapSet(v string) bool {
	v = strings.TrimSpace(v)
	return v != "" && v != "0"
}

func swapSummary(a coolifyApp) string {
	var parts []string
	if swapSet(a.LimitsSwap) {
		parts = append(parts, "swap "+strings.TrimSpace(a.LimitsSwap))
	}
	if swapSet(a.LimitsReservation) {
		parts = append(parts, "reservation "+strings.TrimSpace(a.LimitsReservation))
	}
	return strings.Join(parts, ", ")
}

// ImportCommandMaxEnv overrides the longest build or deploy command an import carries over.
const ImportCommandMaxEnv = "APP_IMPORT_COMMAND_MAX"

const defaultImportCommandMax = 4096

// ImportCommandMax returns the command length cap, from ImportCommandMaxEnv when it is a positive integer.
func ImportCommandMax() int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(ImportCommandMaxEnv))); err == nil && v > 0 {
		return v
	}
	return defaultImportCommandMax
}

// CleanSourcePath turns a source path such as "/apps/web" into a repository relative one.
// A traversal, absolute drive path or control character is rejected, never rewritten.
func CleanSourcePath(field, p string) (string, error) {
	p = strings.TrimSpace(p)
	if strings.ContainsAny(p, "\r\n") {
		return "", fmt.Errorf("%s contains a line break", field)
	}
	return spec.NormalizeRepoPath(field, strings.TrimLeft(p, "/"))
}

// ImportValueMatchMinEnv overrides the shortest fetched env value that a command is checked for.
const ImportValueMatchMinEnv = "APP_IMPORT_VALUE_MATCH_MIN"

const defaultImportValueMatchMin = 4

func importValueMatchMin() int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(ImportValueMatchMinEnv))); err == nil && v > 0 {
		return v
	}
	return defaultImportValueMatchMin
}

const cmdBoundary = "\\s;&|()\"'`"

var (
	commandAssignRe   = regexp.MustCompile(`(?:^|[` + cmdBoundary + `])(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)=([^` + cmdBoundary + `]*)`)
	commandFlagRe     = regexp.MustCompile(`(?i)(?:^|[` + cmdBoundary + `])--?([a-z][a-z0-9_-]*)(?:=|\s+)["']?([^` + cmdBoundary + `]*)`)
	commandShortPRe   = regexp.MustCompile(`(?:^|[` + cmdBoundary + `])-p([^` + cmdBoundary + `-][^` + cmdBoundary + `]*)`)
	commandUserinfoRe = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://[^\s/@"'` + "`" + `]*:[^\s/@"'` + "`" + `]+@`)
	commandHeaderRe   = regexp.MustCompile(`(?i)\b(?:authorization|x-api-key|x-auth-token|private-token)\s*:|\bbearer\s+[A-Za-z0-9._~+/=-]{4,}`)
	// Suffix match without an underscore, so PGPASSWORD and GHTOKEN count too.
	secretNameRe = regexp.MustCompile(`(?i)(pass|passwd|password|pwd|token|secret|api_?key|private_?key|credentials?|auth)$`)
	portValueRe  = regexp.MustCompile(`^[0-9.:]+(?:/(?:tcp|udp))?$`)
	loginRe      = regexp.MustCompile(`(?i)\blogin\b|\bmysql\b|\bmysqldump\b|\bmariadb\b`)
)

func secretName(key string) bool {
	key = strings.ReplaceAll(key, "-", "_")
	return LooksSecret(key) || secretNameRe.MatchString(key)
}

// literalValue is true for a value that is not empty and not a $VAR reference.
func literalValue(v string) bool {
	v = strings.Trim(v, `"'`)
	return v != "" && !strings.HasPrefix(v, "$")
}

// CommandHasSecret reports whether a command carries a credential inline: a secret-looking variable or
// flag, -p<value>, URL userinfo or an auth header, so it is never copied into a spec or printed.
func CommandHasSecret(cmd string) bool {
	if commandUserinfoRe.MatchString(cmd) || commandHeaderRe.MatchString(cmd) {
		return true
	}
	for _, m := range commandAssignRe.FindAllStringSubmatch(cmd, -1) {
		if secretName(m[1]) && literalValue(m[2]) {
			return true
		}
	}
	for _, m := range commandFlagRe.FindAllStringSubmatch(cmd, -1) {
		if len(m[1]) > 1 && secretName(m[1]) && literalValue(m[2]) {
			return true
		}
		// A spaced -p is a password only for login style commands; mkdir -p and docker run -p are not.
		if m[1] == "p" && loginRe.MatchString(cmd) && literalValue(m[2]) && !portValueRe.MatchString(m[2]) {
			return true
		}
	}
	for _, m := range commandShortPRe.FindAllStringSubmatch(cmd, -1) {
		if loginRe.MatchString(cmd) && literalValue(m[1]) && !portValueRe.MatchString(m[1]) {
			return true
		}
	}
	return false
}

// commandHasValue reports whether cmd embeds any fetched env value at least importValueMatchMin long.
func commandHasValue(cmd string, values []string) bool {
	minLen := importValueMatchMin()
	for _, v := range values {
		if v = strings.TrimSpace(v); len(v) >= minLen && v != SecretMask && strings.Contains(cmd, v) {
			return true
		}
	}
	return false
}

// CleanSourceCommand validates one source command: single line, within ImportCommandMax, no inline
// secret and none of the fetched env values. The error never quotes the command.
func CleanSourceCommand(field, cmd string, values ...string) (string, error) {
	cmd = strings.TrimSpace(cmd)
	switch {
	case cmd == "":
		return "", nil
	case strings.ContainsAny(cmd, "\r\n\x00"):
		return "", fmt.Errorf("%s spans more than one line", field)
	case len(cmd) > ImportCommandMax():
		return "", fmt.Errorf("%s is %d bytes, over the cap of %d (%s)", field, len(cmd), ImportCommandMax(), ImportCommandMaxEnv)
	case CommandHasSecret(cmd):
		return "", fmt.Errorf("%s sets a secret-looking value inline", field)
	case commandHasValue(cmd, values):
		return "", fmt.Errorf("%s contains the value of an environment variable", field)
	}
	return cmd, nil
}

// NodePinKeys are the variables the source build packs read to pin the Node version.
var NodePinKeys = []string{"NIXPACKS_NODE_VERSION", "NODE_VERSION"}

// ParseSourceTags reads a tag list given either as strings or as objects with a name.
func ParseSourceTags(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var names []string
	if json.Unmarshal(raw, &names) != nil {
		var objs []struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(raw, &objs) != nil {
			return nil
		}
		for _, o := range objs {
			names = append(names, o.Name)
		}
	}
	var out []string
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// coolifyBuildSettings validates the app's build and deploy settings; each rejected one becomes a note.
func coolifyBuildSettings(a coolifyApp, values []string) (BuildSettings, []Note) {
	var b BuildSettings
	var notes []Note
	reject := func(err error, manual string) {
		notes = append(notes, Note{Area: "build", Reason: err.Error() + ", so it is not carried over", Manual: manual})
	}
	var err error
	if b.BaseDirectory, err = CleanSourcePath("base directory", a.BaseDirectory); err != nil {
		reject(err, "set a base directory inside the repository in the app's build settings")
	}
	if b.DockerfilePath, err = CleanSourcePath("dockerfile location", a.DockerfileLocation); err != nil {
		reject(err, "set a Dockerfile path inside the repository in the app's build settings")
	}
	if b.PublishDir, err = CleanSourcePath("publish directory", a.PublishDirectory); err != nil {
		reject(err, "set the static output directory in the app's build settings")
	}
	cmds := []struct {
		field, val string
		dst        *string
	}{
		{"install command", a.InstallCommand, &b.InstallCommand}, {"build command", a.BuildCommand, &b.BuildCommand},
		{"start command", a.StartCommand, &b.StartCommand}, {"pre-deployment command", a.PreDeployment, &b.PreDeploy},
		{"post-deployment command", a.PostDeployment, &b.PostDeploy},
	}
	for _, c := range cmds {
		if *c.dst, err = CleanSourceCommand(c.field, c.val, values...); err != nil {
			reject(err, "set the "+c.field+" by hand, keep any secret in an app secret and reference it as $NAME")
		}
	}
	return b, notes
}

// buildPlanNotes lists each carried build setting as a planned item, so the report shows what the staged app will use.
func buildPlanNotes(a coolifyApp, b BuildSettings) []Note {
	var notes []Note
	plan := func(reason, manual string) {
		notes = append(notes, Note{Area: "build", Reason: reason, Manual: manual})
	}
	for _, c := range []struct{ name, val string }{{"install", b.InstallCommand}, {"build", b.BuildCommand}, {"start", b.StartCommand}} {
		if c.val != "" {
			plan("planned: "+c.name+" command "+c.val+" is recorded in the build settings", "check it in the app's build settings; Railpack detects "+c.name+" steps itself when it is left empty")
		}
	}
	if b.PublishDir != "" && strings.EqualFold(a.BuildPack, "static") {
		plan("planned: static publish directory "+b.PublishDir+" is recorded in the build settings", "check the static output directory after import")
	}
	if b.PreDeploy != "" || b.PostDeploy != "" {
		plan("planned: pre or post deployment commands are recorded as deploy hooks", "check the deploy hooks in the app settings")
	}
	if tags := ParseSourceTags(a.Tags); len(tags) > 0 {
		plan("planned: source tags "+strings.Join(tags, ", ")+" have no tag field here", "group the apps in one project or name them with a shared prefix")
	}
	if extra := extraPorts(a.PortsExposes); extra != "" {
		notes = append(notes, Note{Area: "network", Reason: "only the first exposed port is routed, extra ports " + extra + " are not imported", Manual: "publish each extra port in the app's port settings"})
	}
	return notes
}

// extraPorts returns every exposed port after the first, comma separated.
func extraPorts(list string) string {
	parts := strings.Split(list, ",")
	if len(parts) < 2 {
		return ""
	}
	var out []string
	for _, p := range parts[1:] {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ",")
}

// nodePinNote reports a Node version pinned through a build pack variable, which Railpack does not read.
func nodePinNote(env []Env) *Note {
	for _, k := range NodePinKeys {
		for _, e := range env {
			if e.Key == k && strings.TrimSpace(e.Value) != "" && !e.Secret {
				return &Note{Area: "build", Reason: "planned: Node is pinned to " + strings.TrimSpace(e.Value) + " through " + k,
					Manual: "pin it in the repository with engines.node in package.json or a .node-version file, which Railpack reads"}
			}
		}
	}
	return nil
}

const composeBuildPack = "application uses the Docker Compose build pack (multi-service)"

// composeApp reads a compose build pack app's env and checks its compose body; nothing is staged here.
func (c *CoolifySource) composeApp(ctx context.Context, project, environment string, a coolifyApp) ComposeApp {
	tmp := App{SourceID: a.UUID, Name: a.Name}
	c.readDetails(ctx, &tmp)
	out := NewComposeApp(a.UUID, a.Name, project, a.DockerComposeRaw, tmp.Env, splitHosts(a.FQDN))
	out.Environment = environment
	out.Notes = append(out.Notes, tmp.Notes...)
	if len(tmp.Files) > 0 {
		out.Notes = append(out.Notes, Note{Area: "file", Reason: "file mounts of a compose app are not copied", Manual: "declare them under configs: in the compose file"})
	}
	return out
}
