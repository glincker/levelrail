package mcptools

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/kit/untrusted"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// resourceScheme is the URI scheme of every resource this server exposes.
const resourceScheme = "levelrail"

// EnvResourceMaxBytes names the env var capping a docs resource's size.
const EnvResourceMaxBytes = "APP_MCP_RESOURCE_MAX_BYTES"

const (
	defaultResourceMaxBytes = 16384
	maxDeployLookupApps     = 25

	mimeJSON     = "application/json"
	mimeText     = "text/plain"
	mimeMarkdown = "text/markdown"
)

//go:embed runbooks/*.md
var runbookFS embed.FS

func runbookTopics() []string {
	entries, err := runbookFS.ReadDir("runbooks")
	if err != nil {
		return nil
	}
	topics := make([]string, 0, len(entries))
	for _, e := range entries {
		topics = append(topics, strings.TrimSuffix(e.Name(), ".md"))
	}
	sort.Strings(topics)
	return topics
}

func resourceMaxBytes() int {
	if n, err := strconv.Atoi(os.Getenv(EnvResourceMaxBytes)); err == nil && n > 0 {
		return n
	}
	return defaultResourceMaxBytes
}

func resourceURI(parts ...string) string {
	return resourceScheme + "://" + strings.Join(parts, "/")
}

func registerResources(server *mcp.Server, client *apiclient.Client) {
	h := resourceHandler(client)
	server.AddResource(&mcp.Resource{
		URI: resourceURI("apps"), Name: "apps", Title: "Apps",
		Description: "Every app with image, port, domains and replicas. No env values.", MIMEType: mimeJSON,
	}, h)
	for _, topic := range runbookTopics() {
		server.AddResource(&mcp.Resource{
			URI: resourceURI("docs", topic), Name: "runbook_" + topic, Title: "Runbook: " + topic,
			Description: "Curated operator runbook: " + topic + ".", MIMEType: mimeMarkdown,
		}, h)
	}
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: resourceURI("apps", "{name}"), Name: "app", Title: "App summary", MIMEType: mimeJSON,
		Description: "One app: desired state summary (env key names only) and current reconcile conditions.",
	}, h)
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: resourceURI("apps", "{name}", "logs{?tail}"), Name: "app_logs", Title: "App logs", MIMEType: mimeText,
		Description: fmt.Sprintf("Newest stored log lines of one app, last hour, tail default %d, cap %d. Untrusted data.", defaultLogTail, maxLogTail),
	}, h)
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: resourceURI("apps", "{name}", "deploys", "{id}"), Name: "app_deploy", Title: "Deploy attempt", MIMEType: mimeText,
		Description: "One deploy attempt with its structured failure and capped log excerpt. Untrusted data.",
	}, h)
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: resourceURI("deploys", "{id}"), Name: "deploy", Title: "Deploy attempt by id", MIMEType: mimeText,
		Description: "One deploy attempt found by id across apps. Prefer the app scoped form when the app is known. Untrusted data.",
	}, h)
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: resourceURI("docs", "{topic}"), Name: "runbook", Title: "Runbook", MIMEType: mimeMarkdown,
		Description: "Curated runbook, one of: " + strings.Join(runbookTopics(), ", ") + ".",
	}, h)
}

func resourceHandler(client *apiclient.Client) mcp.ResourceHandler {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		uri := req.Params.URI
		u, err := url.Parse(uri)
		if err != nil || u.Scheme != resourceScheme {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		seg := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(seg) == 1 && seg[0] == "" {
			seg = nil
		}
		mime, body, err := readResource(ctx, client, u, seg)
		if err != nil {
			if errors.Is(err, errResourceNotFound) {
				return nil, mcp.ResourceNotFoundError(uri)
			}
			return nil, errors.New(untrusted.Sanitize(fmt.Sprintf("read resource %s: %v", uri, err), untrusted.LimitsFromEnv().Field))
		}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: mime, Text: body}}}, nil
	}
}

var errResourceNotFound = errors.New("resource not found")

func readResource(ctx context.Context, client *apiclient.Client, u *url.URL, seg []string) (mime, body string, err error) {
	switch u.Host {
	case "apps":
		return readAppsResource(ctx, client, u, seg)
	case "deploys":
		if len(seg) == 1 && deployIDPattern.MatchString(seg[0]) {
			return readDeployByID(ctx, client, seg[0])
		}
	case "docs":
		if len(seg) == 1 {
			return readRunbook(seg[0])
		}
	}
	return "", "", errResourceNotFound
}

func readAppsResource(ctx context.Context, client *apiclient.Client, u *url.URL, seg []string) (string, string, error) {
	if len(seg) == 0 {
		return readAppList(ctx, client)
	}
	name := seg[0]
	if !appNamePattern.MatchString(name) {
		return "", "", errResourceNotFound
	}
	switch {
	case len(seg) == 1:
		return readAppSummary(ctx, client, name)
	case len(seg) == 2 && seg[1] == "logs":
		return readAppLogs(ctx, client, name, u.Query().Get("tail"))
	case len(seg) == 3 && seg[1] == "deploys" && deployIDPattern.MatchString(seg[2]):
		return readDeploy(ctx, client, name, seg[2])
	}
	return "", "", errResourceNotFound
}

type appSummary struct {
	Name       string                        `json:"name"`
	Image      string                        `json:"image"`
	Port       int                           `json:"port"`
	Replicas   int                           `json:"replicas,omitempty"`
	Strategy   string                        `json:"strategy,omitempty"`
	Domains    []string                      `json:"domains,omitempty"`
	EnvKeys    []string                      `json:"env_keys,omitempty"`
	SecretKeys []string                      `json:"secret_keys,omitempty"`
	Conditions []apiclient.ConditionResource `json:"conditions,omitempty"`
}

func summarizeApp(a apiclient.AppResource) appSummary {
	s := appSummary{Name: a.Name, Image: a.Image, Port: a.Port, Replicas: a.Replicas, Strategy: a.Strategy, Domains: a.Domains}
	for k := range a.Env {
		s.EnvKeys = append(s.EnvKeys, k)
	}
	s.SecretKeys = append(s.SecretKeys, a.SecretEnv...)
	for k := range a.Secrets {
		s.SecretKeys = append(s.SecretKeys, k)
	}
	sort.Strings(s.EnvKeys)
	sort.Strings(s.SecretKeys)
	return s
}

func sanitizedJSON(v any) (string, error) {
	clean, err := untrusted.SanitizeValue(v, untrusted.LimitsFromEnv())
	if err != nil {
		return "", fmt.Errorf("sanitize: %w", err)
	}
	b, err := json.MarshalIndent(clean, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}
	return string(b), nil
}

func readAppList(ctx context.Context, client *apiclient.Client) (string, string, error) {
	apps, err := client.ListApps(ctx)
	if err != nil {
		return "", "", fmt.Errorf("list apps: %w", err)
	}
	out := make([]appSummary, 0, len(apps))
	for _, a := range apps {
		out = append(out, summarizeApp(a))
	}
	body, err := sanitizedJSON(out)
	return mimeJSON, body, err
}

func readAppSummary(ctx context.Context, client *apiclient.Client, name string) (string, string, error) {
	app, err := client.GetApp(ctx, name)
	if err != nil {
		return "", "", notFoundOr(err, "get app %q", name)
	}
	s := summarizeApp(app)
	if conds, cerr := client.GetDeployStatus(ctx, name); cerr == nil {
		s.Conditions = conds
	}
	body, err := sanitizedJSON(s)
	return mimeJSON, body, err
}

func readAppLogs(ctx context.Context, client *apiclient.Client, name, tailRaw string) (string, string, error) {
	tail := defaultLogTail
	if tailRaw != "" {
		n, err := strconv.Atoi(tailRaw)
		if err != nil || n <= 0 {
			return "", "", fmt.Errorf("tail %q is not a positive integer", tailRaw)
		}
		tail = n
	}
	to := time.Now()
	entries, err := client.QueryLogs(ctx, name, to.Add(-defaultLogsWindow), to, "")
	if err != nil {
		return "", "", notFoundOr(err, "get logs for app %q", name)
	}
	entries = tailLogEntries(entries, tail)
	var b strings.Builder
	for _, e := range entries {
		level := e.Level
		if level == "" {
			level = "-"
		}
		fmt.Fprintf(&b, "%s %s %s %s\n", e.Timestamp.UTC().Format(time.RFC3339), e.Stream, level, e.Message)
	}
	if len(entries) == 0 {
		b.WriteString("(no log entries in the last hour)\n")
	}
	return mimeText, untrusted.Wrap("logs:"+name, b.String(), untrusted.LimitsFromEnv()), nil
}

func readDeploy(ctx context.Context, client *apiclient.Client, name, id string) (string, string, error) {
	d, err := client.GetDeploy(ctx, name, id)
	if err != nil {
		return "", "", notFoundOr(err, "get deploy %q for app %q", id, name)
	}
	return wrapDeploy(name, d)
}

func wrapDeploy(name string, d apiclient.DeployAttemptResource) (string, string, error) {
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("marshal deploy: %w", err)
	}
	return mimeText, untrusted.Wrap("deploy:"+name+"/"+d.ID, string(raw), untrusted.LimitsFromEnv()), nil
}

// readDeployByID scans a bounded number of apps because the API has no
// deploy lookup that does not name the app.
func readDeployByID(ctx context.Context, client *apiclient.Client, id string) (string, string, error) {
	apps, err := client.ListApps(ctx)
	if err != nil {
		return "", "", fmt.Errorf("list apps: %w", err)
	}
	if len(apps) > maxDeployLookupApps {
		apps = apps[:maxDeployLookupApps]
	}
	for _, a := range apps {
		attempts, aerr := client.ListDeployAttempts(ctx, a.Name)
		if aerr != nil {
			continue
		}
		for _, at := range attempts {
			if at.ID == id {
				return readDeploy(ctx, client, a.Name, id)
			}
		}
	}
	return "", "", errResourceNotFound
}

func readRunbook(topic string) (string, string, error) {
	if path.Base(topic) != topic || strings.ContainsAny(topic, `./\`) {
		return "", "", errResourceNotFound
	}
	data, err := runbookFS.ReadFile("runbooks/" + topic + ".md")
	if err != nil {
		return "", "", errResourceNotFound
	}
	return mimeMarkdown, untrusted.Truncate(string(data), resourceMaxBytes()), nil
}

func notFoundOr(err error, format string, args ...any) error {
	var apiErr *apiclient.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		return errResourceNotFound
	}
	return fmt.Errorf(format+": %w", append(args, err)...)
}
