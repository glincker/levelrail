package platformimport

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// DefaultHerokuURL is the public Heroku Platform API.
const DefaultHerokuURL = "https://api.heroku.com"

// herokuPageSize is the largest page the Platform API serves for /apps.
const herokuPageSize = 1000

// HerokuSource reads a Heroku account through the Platform API, GET only.
type HerokuSource struct {
	r *requester
}

// NewHeroku builds a Heroku source. The API token is kept in memory only.
// An empty baseURL means the public API.
func NewHeroku(baseURL, token string, o ClientOptions) (*HerokuSource, error) {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultHerokuURL
	}
	r, err := newRequester(baseURL, o, token)
	if err != nil {
		return nil, err
	}
	r.headers["Authorization"] = "Bearer " + token
	r.headers["Accept"] = "application/vnd.heroku+json; version=3"
	return &HerokuSource{r: r}, nil
}

type herokuApp struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Region struct {
		Name string `json:"name"`
	} `json:"region"`
	Team *struct {
		Name string `json:"name"`
	} `json:"team"`
	Stack struct {
		Name string `json:"name"`
	} `json:"stack"`
}

type herokuProcess struct {
	Type     string `json:"type"`
	Quantity int    `json:"quantity"`
	Size     string `json:"size"`
	Command  string `json:"command"`
}

type herokuDomain struct {
	Hostname string `json:"hostname"`
	Kind     string `json:"kind"`
}

type herokuAddon struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	AddonService struct {
		Name string `json:"name"`
	} `json:"addon_service"`
	Plan struct {
		Name string `json:"name"`
	} `json:"plan"`
	ConfigVars []string `json:"config_vars"`
}

// Discover lists apps, then reads config, formation, domains and add-ons for each.
func (s *HerokuSource) Discover(ctx context.Context) (*Discovery, error) {
	var apps []herokuApp
	// The default page is 200 apps, so ask for the largest one the API allows.
	s.r.headers["Range"] = fmt.Sprintf("id ..; max=%d", herokuPageSize)
	err := s.r.get(ctx, "/apps", nil, &apps)
	delete(s.r.headers, "Range")
	if err != nil {
		return nil, fmt.Errorf("list heroku apps: %w", err)
	}
	d := &Discovery{Platform: Heroku}
	if len(apps) >= herokuPageSize {
		d.Warnings = append(d.Warnings, fmt.Sprintf("the account has at least %d apps and only the first %d were read, use --only after splitting by team", herokuPageSize, herokuPageSize))
	}
	for _, a := range apps {
		s.collect(ctx, d, a)
	}
	return d, nil
}

func (s *HerokuSource) collect(ctx context.Context, d *Discovery, a herokuApp) {
	base := "/apps/" + url.PathEscape(a.ID)
	soft := func(what string, err error) {
		d.Warnings = append(d.Warnings, fmt.Sprintf("could not read %s of app %s (%s): that detail is missing from this report", what, a.Name, err.Error()))
	}
	var vars map[string]string
	if err := s.r.get(ctx, base+"/config-vars", nil, &vars); err != nil {
		soft("config vars", err)
	}
	var formation []herokuProcess
	if err := s.r.get(ctx, base+"/formation", nil, &formation); err != nil {
		soft("the process formation", err)
	}
	var domains []herokuDomain
	if err := s.r.get(ctx, base+"/domains", nil, &domains); err != nil {
		soft("domains", err)
	}
	var addons []herokuAddon
	if err := s.r.get(ctx, base+"/addons", nil, &addons); err != nil {
		soft("add-ons", err)
	}

	project := ""
	if a.Team != nil {
		project = a.Team.Name
	}
	app := App{SourceID: a.ID, Name: a.Name, Project: project, Server: a.Region.Name, Kind: SourceUnknown, Replicas: 1}
	if a.Stack.Name != "" {
		app.BuildPack = a.Stack.Name
	}
	addonVars := map[string]bool{}
	for _, ad := range addons {
		for _, k := range ad.ConfigVars {
			addonVars[k] = true
		}
	}
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		app.Env = append(app.Env, Env{Key: k, Value: vars[k], Secret: LooksSecret(k) || addonVars[k]})
	}
	for _, dm := range domains {
		if dm.Kind != "heroku" && dm.Hostname != "" {
			app.Domains = append(app.Domains, strings.ToLower(dm.Hostname))
		}
	}
	app.Notes = append(app.Notes,
		Note{Reason: "Heroku keeps no repository for the app, its code only exists as slugs there", Manual: "import the repository with `import <repo>` and attach it to this app"},
		Note{Reason: "Heroku injects PORT at runtime", Manual: "make sure the app listens on the port set after import"})

	var workers []App
	for _, p := range formation {
		if p.Quantity < 1 {
			continue
		}
		if p.Type == "web" {
			app.Replicas = p.Quantity
			continue
		}
		w := App{SourceID: a.ID + "/" + p.Type, Name: a.Name + "-" + p.Type, Project: project, Server: a.Region.Name, Kind: SourceUnknown,
			Replicas: p.Quantity, Env: app.Env, Notes: []Note{
				{Reason: fmt.Sprintf("Heroku %s process of %s, dyno size %s, not exposed over HTTP", p.Type, a.Name, p.Size), Manual: "create it as a worker from the same repository with the command from the Procfile"},
				app.Notes[0],
			}}
		workers = append(workers, w)
	}
	d.Apps = append(d.Apps, app)
	d.Apps = append(d.Apps, workers...)

	for _, ad := range addons {
		s.mapAddon(d, a, ad)
	}
}

func (s *HerokuSource) mapAddon(d *Discovery, a herokuApp, ad herokuAddon) {
	svc := ad.AddonService.Name
	engine := ""
	switch svc {
	case "heroku-postgresql":
		engine = "postgres"
	case "heroku-redis":
		engine = "redis"
	}
	if engine == "" {
		d.Warnings = append(d.Warnings, fmt.Sprintf("no managed equivalent, set up manually: %s (%s on app %s)", svc, ad.Plan.Name, a.Name))
		return
	}
	// The add-on API reports no engine version, so the plan flags it for a manual pick.
	d.Databases = append(d.Databases, Database{SourceID: ad.ID, Name: ad.Name, Project: a.Name, Engine: engine})
	d.Warnings = append(d.Warnings, fmt.Sprintf("%s add-on %s on app %s uses plan %s: pick a matching size here, data is not migrated", svc, ad.Name, a.Name, ad.Plan.Name))
}
