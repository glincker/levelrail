package platformimport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// DefaultVercelURL is the public Vercel REST API.
const DefaultVercelURL = "https://api.vercel.com"

// vercelPageSize is the largest page the project and domain lists serve.
const vercelPageSize = 100

// vercelMaxPages bounds pagination so a misbehaving server cannot loop forever.
const vercelMaxPages = 200

// VercelSource reads a Vercel account or team through the REST API, GET only.
type VercelSource struct {
	r *requester
	// TeamID scopes every call to a team, empty means the token's personal account.
	TeamID string
}

// NewVercel builds a Vercel source. The API token is kept in memory only.
// An empty baseURL means the public API.
func NewVercel(baseURL, token string, o ClientOptions) (*VercelSource, error) {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultVercelURL
	}
	r, err := newRequester(baseURL, o, token)
	if err != nil {
		return nil, err
	}
	r.headers["Authorization"] = "Bearer " + token
	src := &VercelSource{r: r}
	if o.Team != "" {
		team, err := ValidateVercelTeam(o.Team)
		if err != nil {
			return nil, err
		}
		src.WithTeam(team)
	}
	return src, nil
}

// WithTeam scopes the source to one team id (team_...).
func (s *VercelSource) WithTeam(id string) *VercelSource {
	s.TeamID = strings.TrimSpace(id)
	return s
}

// vercelTeamPattern accepts a team id (team_...) or a team slug.
var vercelTeamPattern = regexp.MustCompile(`^(team_[A-Za-z0-9]{4,64}|[a-z0-9](?:[a-z0-9_-]{0,62}[a-z0-9])?)$`)

// ValidateVercelTeam trims and checks a team id or slug before it reaches a query string.
func ValidateVercelTeam(s string) (string, error) {
	s = strings.TrimSpace(s)
	if !vercelTeamPattern.MatchString(s) {
		return "", fmt.Errorf("team_id must be a Vercel team id (team_...) or a team slug")
	}
	return s, nil
}

type vercelPagination struct {
	// Next is a timestamp or a continuation token depending on the endpoint.
	Next json.RawMessage `json:"next"`
}

func (p vercelPagination) next() string {
	s := strings.Trim(strings.TrimSpace(string(p.Next)), `"`)
	if s == "null" {
		return ""
	}
	return s
}

type vercelProject struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Framework       string `json:"framework"`
	BuildCommand    string `json:"buildCommand"`
	InstallCommand  string `json:"installCommand"`
	OutputDirectory string `json:"outputDirectory"`
	RootDirectory   string `json:"rootDirectory"`
	NodeVersion     string `json:"nodeVersion"`
	Link            *struct {
		Type             string `json:"type"`
		Org              string `json:"org"`
		Repo             string `json:"repo"`
		ProductionBranch string `json:"productionBranch"`
	} `json:"link"`
	Crons json.RawMessage `json:"crons"`
}

type vercelEnv struct {
	Key    string          `json:"key"`
	Type   string          `json:"type"`
	Value  string          `json:"value"`
	Target json.RawMessage `json:"target"`
}

// targets accepts the documented array form and the single-string form.
func (e vercelEnv) targets() []string {
	var many []string
	if json.Unmarshal(e.Target, &many) == nil {
		return many
	}
	var one string
	if json.Unmarshal(e.Target, &one) == nil && one != "" {
		return []string{one}
	}
	return nil
}

type vercelDomain struct {
	Name     string `json:"name"`
	Redirect string `json:"redirect"`
}

func (s *VercelSource) query(extra url.Values) url.Values {
	q := url.Values{}
	for k, v := range extra {
		q[k] = v
	}
	if s.TeamID != "" {
		q.Set("teamId", s.TeamID)
	}
	return q
}

// Discover lists projects, then reads env keys and domains for each.
func (s *VercelSource) Discover(ctx context.Context) (*Discovery, error) {
	d := &Discovery{Platform: Vercel}
	from := ""
	for page := 0; page < vercelMaxPages; page++ {
		q := url.Values{"limit": {strconv.Itoa(vercelPageSize)}}
		if from != "" {
			q.Set("from", from)
		}
		var resp struct {
			Projects   []vercelProject  `json:"projects"`
			Pagination vercelPagination `json:"pagination"`
		}
		if err := s.r.get(ctx, "/v10/projects", s.query(q), &resp); err != nil {
			return nil, fmt.Errorf("list vercel projects: %w", err)
		}
		for _, p := range resp.Projects {
			s.collect(ctx, d, p)
		}
		from = resp.Pagination.next()
		if from == "" {
			return d, nil
		}
	}
	d.Warnings = append(d.Warnings, fmt.Sprintf("stopped after %d pages of projects, the rest were not read", vercelMaxPages))
	return d, nil
}

func (s *VercelSource) collect(ctx context.Context, d *Discovery, p vercelProject) {
	base := "/v10/projects/" + url.PathEscape(p.ID)
	soft := func(what string, err error) {
		d.Warnings = append(d.Warnings, fmt.Sprintf("could not read %s of project %s (%s): that detail is missing from this report", what, p.Name, err.Error()))
	}
	// decrypt is left off so the API never returns a readable secret.
	var envs struct {
		Envs []vercelEnv `json:"envs"`
	}
	if err := s.r.get(ctx, base+"/env", s.query(url.Values{"decrypt": {"false"}}), &envs); err != nil {
		soft("environment variables", err)
	}
	domains, err := s.domains(ctx, p.ID)
	if err != nil {
		soft("domains", err)
	}

	app := App{SourceID: p.ID, Name: p.Name, Kind: SourceUnknown, Replicas: 1, BuildPack: p.Framework, BuildPath: strings.Trim(p.RootDirectory, "/")}
	if l := p.Link; l != nil && l.Org != "" && l.Repo != "" {
		if host := vercelGitHost(l.Type); host != "" {
			app.Kind, app.GitURL, app.GitBranch = SourceGit, "https://"+host+"/"+l.Org+"/"+l.Repo, l.ProductionBranch
		}
	}
	if app.Kind != SourceGit {
		app.Notes = append(app.Notes, Note{Reason: "Vercel project is not linked to a repository this importer can resolve", Manual: "import the repository with `import <repo>` and attach it to this app"})
	}
	for _, c := range []struct{ label, v string }{
		{"build command", p.BuildCommand}, {"install command", p.InstallCommand},
		{"output directory", p.OutputDirectory}, {"Node.js version", p.NodeVersion},
	} {
		if c.v != "" {
			app.Notes = append(app.Notes, Note{Area: "build", Reason: fmt.Sprintf("Vercel %s is %q", c.label, c.v), Manual: "set the same value in the build settings after import"})
		}
	}
	if len(p.Crons) > 2 && string(p.Crons) != "null" {
		d.Warnings = append(d.Warnings, fmt.Sprintf("project %s reports cron jobs on the Vercel side, check them manually", p.Name))
	} else {
		d.Warnings = append(d.Warnings, fmt.Sprintf("crons of project %s are not read from the API, they come from vercel.json when the repository is imported", p.Name))
	}

	app.Env = append(app.Env, s.mapEnv(d, p.Name, envs.Envs)...)
	for _, dm := range domains {
		n := strings.ToLower(dm.Name)
		if n == "" || dm.Redirect != "" || n == "vercel.app" || strings.HasSuffix(n, ".vercel.app") {
			continue
		}
		app.Domains = append(app.Domains, n)
	}
	d.Apps = append(d.Apps, app)
	d.Warnings = append(d.Warnings, fmt.Sprintf("connected Vercel storage of project %s is not discoverable here, check it manually: Postgres maps to a Levelrail Postgres database, KV (Redis) to a Levelrail Redis database, Blob needs an S3 compatible bucket, Edge Config has no equivalent (none)", p.Name))
}

func (s *VercelSource) domains(ctx context.Context, id string) ([]vercelDomain, error) {
	var all []vercelDomain
	until := ""
	for page := 0; page < vercelMaxPages; page++ {
		q := url.Values{"limit": {strconv.Itoa(vercelPageSize)}}
		if until != "" {
			q.Set("until", until)
		}
		var resp struct {
			Domains    []vercelDomain   `json:"domains"`
			Pagination vercelPagination `json:"pagination"`
		}
		if err := s.r.get(ctx, "/v9/projects/"+url.PathEscape(id)+"/domains", s.query(q), &resp); err != nil {
			return all, err
		}
		all = append(all, resp.Domains...)
		until = resp.Pagination.next()
		if until == "" {
			break
		}
	}
	return all, nil
}

// mapEnv keeps one entry per key, imports keys only for non-plain types and
// warns when a key is not set for all three targets.
func (s *VercelSource) mapEnv(d *Discovery, project string, envs []vercelEnv) []Env {
	byKey := map[string][]vercelEnv{}
	for _, e := range envs {
		if e.Key != "" {
			byKey[e.Key] = append(byKey[e.Key], e)
		}
	}
	keys := make([]string, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []Env
	var unreadable, partial []string
	for _, k := range keys {
		group := byKey[k]
		pick := group[0]
		cover := map[string]bool{}
		for _, e := range group {
			for _, t := range e.targets() {
				cover[t] = true
				if t == "production" {
					pick = e
				}
			}
		}
		if len(group) > 1 || (!cover["production"] || !cover["preview"] || !cover["development"]) {
			var ts []string
			for t := range cover {
				ts = append(ts, t)
			}
			sort.Strings(ts)
			partial = append(partial, fmt.Sprintf("%s (%s)", k, strings.Join(ts, "+")))
		}
		if pick.Type == "plain" {
			out = append(out, Env{Key: k, Value: pick.Value, Secret: LooksSecret(k)})
			continue
		}
		// encrypted, sensitive, secret and system values are never readable.
		out = append(out, Env{Key: k, Secret: true})
		unreadable = append(unreadable, k)
	}
	if len(unreadable) > 0 {
		d.Warnings = append(d.Warnings, fmt.Sprintf("project %s: value not readable, set it manually: %s", project, strings.Join(unreadable, ", ")))
	}
	if len(partial) > 0 {
		d.Warnings = append(d.Warnings, fmt.Sprintf("project %s: env vars differ by target (production, preview, development), the production value was used: %s", project, strings.Join(partial, ", ")))
	}
	return out
}

func vercelGitHost(t string) string {
	switch t {
	case "github":
		return "github.com"
	case "gitlab":
		return "gitlab.com"
	case "bitbucket":
		return "bitbucket.org"
	}
	return ""
}
