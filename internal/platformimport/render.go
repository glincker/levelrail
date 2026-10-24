package platformimport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// DefaultRenderURL is the public Render REST API.
const DefaultRenderURL = "https://api.render.com/v1"

// renderPageSize is the largest page the list endpoints serve.
const renderPageSize = 100

// renderMaxPages bounds pagination so a misbehaving server cannot loop forever.
const renderMaxPages = 200

// RenderSource reads a Render workspace through the REST API, GET only.
type RenderSource struct {
	r *requester
}

// NewRender builds a Render source. The API key is kept in memory only.
// An empty baseURL means the public API.
func NewRender(baseURL, token string, o ClientOptions) (*RenderSource, error) {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultRenderURL
	}
	r, err := newRequester(baseURL, o, token)
	if err != nil {
		return nil, err
	}
	r.headers["Authorization"] = "Bearer " + token
	r.headers["Accept"] = "application/json"
	return &RenderSource{r: r}, nil
}

type renderDetails struct {
	Env                string      `json:"env"`
	Runtime            string      `json:"runtime"`
	Plan               string      `json:"plan"`
	Region             string      `json:"region"`
	NumInstances       int         `json:"numInstances"`
	HealthCheckPath    string      `json:"healthCheckPath"`
	Schedule           string      `json:"schedule"`
	BuildCommand       string      `json:"buildCommand"`
	PublishPath        string      `json:"publishPath"`
	Disk               *renderDisk `json:"disk"`
	EnvSpecificDetails struct {
		DockerCommand  string `json:"dockerCommand"`
		DockerfilePath string `json:"dockerfilePath"`
		BuildCommand   string `json:"buildCommand"`
		StartCommand   string `json:"startCommand"`
	} `json:"envSpecificDetails"`
}

type renderDisk struct {
	Name      string `json:"name"`
	SizeGB    int    `json:"sizeGB"`
	MountPath string `json:"mountPath"`
}

type renderService struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Type        string        `json:"type"`
	Repo        string        `json:"repo"`
	Branch      string        `json:"branch"`
	RootDir     string        `json:"rootDir"`
	ImagePath   string        `json:"imagePath"`
	AutoDeploy  string        `json:"autoDeploy"`
	Suspended   string        `json:"suspended"`
	Environment string        `json:"environmentId"`
	Details     renderDetails `json:"serviceDetails"`
}

type renderEnvVar struct {
	Key           string `json:"key"`
	Value         string `json:"value"`
	GenerateValue bool   `json:"generateValue"`
}

type renderDomain struct {
	Name             string `json:"name"`
	RedirectForName  string `json:"redirectForName"`
	VerificationStat string `json:"verificationStatus"`
}

type renderPostgres struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Version    string `json:"version"`
	Plan       string `json:"plan"`
	DiskSizeGB int    `json:"diskSizeGB"`
	Region     string `json:"region"`
	DBName     string `json:"databaseName"`
	DBUser     string `json:"databaseUser"`
}

type renderKeyValue struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Plan    string `json:"plan"`
	Region  string `json:"region"`
	Version string `json:"version"`
}

// renderPage reads every page of a cursor-paginated list. Each item is wrapped as
// {"<key>": {...}, "cursor": "..."}; the last cursor of a page starts the next.
func renderPage[T any](ctx context.Context, r *requester, path, key string, extra url.Values) ([]T, error) {
	var all []T
	cursor := ""
	for page := 0; page < renderMaxPages; page++ {
		q := url.Values{"limit": {strconv.Itoa(renderPageSize)}}
		for k, v := range extra {
			q[k] = v
		}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		var raw []map[string]json.RawMessage
		if err := r.get(ctx, path, q, &raw); err != nil {
			return all, err
		}
		if len(raw) == 0 {
			return all, nil
		}
		for _, item := range raw {
			var v T
			if err := json.Unmarshal(item[key], &v); err != nil {
				return all, fmt.Errorf("decode %s item: %w", key, err)
			}
			all = append(all, v)
		}
		cursor = ""
		_ = json.Unmarshal(raw[len(raw)-1]["cursor"], &cursor)
		if cursor == "" {
			return all, nil
		}
	}
	return all, fmt.Errorf("stopped after %d pages of %s", renderMaxPages, path)
}

// Discover lists services, then reads env vars and domains for each, then the datastores.
func (s *RenderSource) Discover(ctx context.Context) (*Discovery, error) {
	svcs, err := renderPage[renderService](ctx, s.r, "/services", "service", nil)
	if err != nil {
		return nil, fmt.Errorf("list render services: %w", err)
	}
	d := &Discovery{Platform: Render}
	for _, svc := range svcs {
		s.collect(ctx, d, svc)
	}
	pgs, err := renderPage[renderPostgres](ctx, s.r, "/postgres", "postgres", nil)
	if err != nil {
		d.Warnings = append(d.Warnings, fmt.Sprintf("could not read Render Postgres databases (%s): they are missing from this report", err.Error()))
	}
	for _, p := range pgs {
		d.Databases = append(d.Databases, Database{SourceID: p.ID, Name: p.Name, Engine: "postgres", Version: p.Version, Server: p.Region, User: p.DBUser, DBName: p.DBName})
		d.Warnings = append(d.Warnings, fmt.Sprintf("Render Postgres %s uses plan %s with %d GB of disk: pick a matching size here, data is not migrated", p.Name, p.Plan, p.DiskSizeGB))
	}
	kvs, err := renderPage[renderKeyValue](ctx, s.r, "/key-value", "keyValue", nil)
	if err != nil {
		d.Warnings = append(d.Warnings, fmt.Sprintf("could not read Render Key Value instances (%s): they are missing from this report", err.Error()))
	}
	for _, k := range kvs {
		// Key Value reports its own version scheme, so the plan picks a Redis version.
		d.Databases = append(d.Databases, Database{SourceID: k.ID, Name: k.Name, Engine: "redis", Server: k.Region})
		d.Warnings = append(d.Warnings, fmt.Sprintf("Render Key Value %s uses plan %s: pick a matching size and Redis version here, data is not migrated", k.Name, k.Plan))
	}
	return d, nil
}

func (s *RenderSource) collect(ctx context.Context, d *Discovery, svc renderService) {
	det := svc.Details
	if svc.Type == "cron_job" {
		cmd := det.EnvSpecificDetails.StartCommand
		if cmd == "" {
			cmd = det.EnvSpecificDetails.DockerCommand
		}
		d.Warnings = append(d.Warnings, fmt.Sprintf("Render cron job %s runs %q on schedule %q: create it as a scheduled task on the app that holds the same code", svc.Name, cmd, det.Schedule))
		return
	}
	soft := func(what string, err error) {
		d.Warnings = append(d.Warnings, fmt.Sprintf("could not read %s of service %s (%s): that detail is missing from this report", what, svc.Name, err.Error()))
	}
	base := "/services/" + url.PathEscape(svc.ID)
	envs, err := renderPage[renderEnvVar](ctx, s.r, base+"/env-vars", "envVar", nil)
	if err != nil {
		soft("environment variables", err)
	}
	domains, err := renderPage[renderDomain](ctx, s.r, base+"/custom-domains", "customDomain", nil)
	if err != nil {
		soft("domains", err)
	}

	app := App{SourceID: svc.ID, Name: svc.Name, Server: det.Region, Kind: SourceUnknown, Replicas: 1, BuildPack: det.Runtime}
	if det.Runtime == "" {
		app.BuildPack = det.Env
	}
	if det.NumInstances > 0 {
		app.Replicas = det.NumInstances
	}
	switch {
	case svc.ImagePath != "":
		app.Kind, app.Image = SourceImage, svc.ImagePath
	case svc.Repo != "":
		app.Kind, app.GitURL, app.GitBranch = SourceGit, svc.Repo, svc.Branch
		app.BuildPath = strings.Trim(svc.RootDir, "/")
	default:
		app.Notes = append(app.Notes, Note{Reason: "Render service has no repository or image path", Manual: "import the repository with `import <repo>` and attach it to this app"})
	}
	switch app.BuildPack {
	case "docker":
		app.BuildMethod = "dockerfile"
		if p := det.EnvSpecificDetails.DockerfilePath; p != "" && app.Kind == SourceGit {
			app.BuildPath = strings.Trim(p, "/")
		}
	}
	note := func(area, reason, manual string) {
		app.Notes = append(app.Notes, Note{Area: area, Reason: reason, Manual: manual})
	}
	switch svc.Type {
	case "static_site":
		app.BuildMethod = "static"
		app.BuildPack = "static"
		if det.BuildCommand != "" {
			note("build", fmt.Sprintf("Render build command is %q", det.BuildCommand), "set the same build command after import")
		}
		if det.PublishPath != "" {
			note("build", fmt.Sprintf("Render publish directory is %q", det.PublishPath), "serve the same directory after import")
		}
	case "background_worker":
		note("", "Render background worker, not exposed over HTTP", "create it as a worker from the same source with the start command below")
	case "private_service":
		note("", "Render private service, only reachable inside the Render network", "keep it off the public routes after import")
	}
	if c := det.EnvSpecificDetails.BuildCommand; c != "" {
		note("build", fmt.Sprintf("Render build command is %q", c), "set the same build command after import")
	}
	if c := det.EnvSpecificDetails.StartCommand; c != "" {
		note("command", fmt.Sprintf("Render start command is %q", c), "set the same start command after import")
	}
	if c := det.EnvSpecificDetails.DockerCommand; c != "" {
		note("command", fmt.Sprintf("Render docker command is %q", c), "set the same command after import")
	}
	if svc.Type != "static_site" && app.Kind == SourceGit && svc.RootDir != "" && app.BuildPath != strings.Trim(svc.RootDir, "/") {
		note("build", fmt.Sprintf("Render root directory is %q", svc.RootDir), "set the same build path after import")
	}
	if svc.AutoDeploy == "yes" {
		note("", "Render auto-deploys on every push", "connect the repository webhook after import")
	}
	if det.Plan != "" {
		note("", fmt.Sprintf("Render plan is %s", det.Plan), "pick a matching memory and CPU size after import")
	}
	if det.HealthCheckPath != "" {
		app.Health = &HealthCheck{Path: det.HealthCheckPath}
	}
	if dk := det.Disk; dk != nil && dk.MountPath != "" {
		app.Volumes = append(app.Volumes, Volume{Name: dk.Name, ContainerPath: dk.MountPath, SizeBytes: int64(dk.SizeGB) << 30})
	}

	sort.SliceStable(envs, func(i, j int) bool { return envs[i].Key < envs[j].Key })
	var generated []string
	for _, e := range envs {
		if e.Key == "" {
			continue
		}
		if e.GenerateValue {
			// Render generates these server side and never reports them.
			app.Env = append(app.Env, Env{Key: e.Key, Secret: true})
			generated = append(generated, e.Key)
			continue
		}
		app.Env = append(app.Env, Env{Key: e.Key, Value: e.Value, Secret: LooksSecret(e.Key)})
	}
	if len(generated) > 0 {
		d.Warnings = append(d.Warnings, fmt.Sprintf("service %s: value generated by Render and not readable, set it manually: %s", svc.Name, strings.Join(generated, ", ")))
	}
	for _, dm := range domains {
		n := strings.ToLower(dm.Name)
		if n == "" || n == "onrender.com" || strings.HasSuffix(n, ".onrender.com") {
			continue
		}
		app.Domains = append(app.Domains, n)
	}
	d.Apps = append(d.Apps, app)
}
