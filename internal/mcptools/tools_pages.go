package mcptools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/apiclient"
	"github.com/glincker/levelrail/internal/sourceupload"
	"github.com/glincker/levelrail/internal/spec"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	envPagesUploadMax = "APP_UPLOAD_MAX_BYTES"
	envPagesInlineMax = "APP_MCP_PAGES_INLINE_MAX_BYTES"
	envPagesWaitSecs  = "APP_MCP_PAGES_WAIT_SECONDS"

	defaultPagesUploadMax = int64(512 << 20)
	defaultPagesInlineMax = int64(1 << 20)
	defaultPagesWaitSecs  = 120
)

// pagesInput is the write input; pagesReadInput is the smaller read one, to keep tools/list lean.
type pagesInput struct {
	Action  string            `json:"action" jsonschema:"create, deploy, domain_add, rollback, delete"`
	Name    string            `json:"name,omitempty" jsonschema:"site name"`
	Dir     string            `json:"dir,omitempty" jsonschema:"deploy: folder"`
	Files   map[string]string `json:"files,omitempty" jsonschema:"deploy: path to text"`
	Domain  string            `json:"domain,omitempty"`
	Deploy  string            `json:"deploy_id,omitempty" jsonschema:"rollback target"`
	Confirm string            `json:"confirm,omitempty" jsonschema:"must equal name"`
}

type pagesReadInput struct {
	Action string `json:"action" jsonschema:"list, show, checks, traffic, domain_status"`
	Name   string `json:"name,omitempty"`
	Domain string `json:"domain,omitempty"`
	Window string `json:"window,omitempty"`
}

type pagesOutput struct {
	Action   string `json:"action"`
	Data     any    `json:"data,omitempty"`
	DeployID string `json:"deploy_id,omitempty"`
	Status   string `json:"status,omitempty"`
	URL      string `json:"url,omitempty"`
	Files    int    `json:"files,omitempty"`
	Note     string `json:"note,omitempty"`
}

func pagesEnvInt(key string, def int64) int64 {
	if n, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(key)), 10, 64); err == nil && n > 0 {
		return n
	}
	return def
}

func registerPagesTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "pages",
		Description: "Read Pages static sites.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in pagesReadInput) (*mcp.CallToolResult, pagesOutput, error) {
		out, err := pagesRead(ctx, client, in)
		return nil, out, err
	})
	addTool(server, &mcp.Tool{
		Name:        "pages_manage",
		Description: "Change Pages; deploy publishes dir or files and returns the url.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in pagesInput) (*mcp.CallToolResult, pagesOutput, error) {
		out, err := pagesWrite(ctx, client, in)
		return nil, out, err
	})
}

func pagesNeedName(action, name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("pages %s: name is required", action)
	}
	return nil
}

func pagesRead(ctx context.Context, c *apiclient.Client, in pagesReadInput) (pagesOutput, error) {
	out := pagesOutput{Action: in.Action}
	var err error
	switch in.Action {
	case "list":
		out.Data, err = c.ListStaticSites(ctx)
	case "show", "checks", "traffic", "domain_status":
		if err = pagesNeedName(in.Action, in.Name); err != nil {
			return out, err
		}
		switch in.Action {
		case "show":
			var s apiclient.StaticSiteDetail
			s, err = c.GetStaticSite(ctx, in.Name)
			out.Data = s
		case "checks":
			var r apiclient.StaticSiteChecks
			r, err = c.StaticSiteChecks(ctx, in.Name)
			out.Data = r
		case "traffic":
			if in.Window != "" && in.Window != "24h" && in.Window != "7d" {
				return out, errors.New("pages traffic: window must be 24h or 7d")
			}
			out.Data, err = c.StaticSiteTraffic(ctx, in.Name, in.Window)
		default:
			if strings.TrimSpace(in.Domain) == "" {
				return out, errors.New("pages domain_status: domain is required")
			}
			var d apiclient.StaticSiteDomain
			d, err = c.StaticSiteDomainStatus(ctx, in.Name, in.Domain)
			out.Data = d
		}
	default:
		return out, fmt.Errorf("pages: unknown action %q, want list, show, checks, traffic or domain_status", in.Action)
	}
	if err != nil {
		return out, fmt.Errorf("pages %s: %w", in.Action, err)
	}
	return out, nil
}

func pagesWrite(ctx context.Context, c *apiclient.Client, in pagesInput) (pagesOutput, error) {
	out := pagesOutput{Action: in.Action}
	switch in.Action {
	case "create":
		if err := pagesNeedName(in.Action, in.Name); err != nil {
			return out, err
		}
		var domains []string
		if in.Domain != "" {
			domains = []string{in.Domain}
		}
		s, err := c.CreateStaticSite(ctx, in.Name, domains)
		if err != nil {
			return out, fmt.Errorf("pages create: %w", err)
		}
		out.Data = s
	case "domain_add":
		if err := pagesNeedName(in.Action, in.Name); err != nil {
			return out, err
		}
		if strings.TrimSpace(in.Domain) == "" {
			return out, errors.New("pages domain_add: domain is required")
		}
		d, err := c.AddStaticSiteDomain(ctx, in.Name, in.Domain)
		if err != nil {
			return out, fmt.Errorf("pages domain_add: %w", err)
		}
		out.Data = d
	case "deploy":
		return pagesDeploy(ctx, c, in)
	case "rollback":
		return pagesRollback(ctx, c, in)
	case "delete":
		if err := pagesNeedName(in.Action, in.Name); err != nil {
			return out, err
		}
		if err := requireConfirm("pages delete", "name", in.Name, in.Confirm); err != nil {
			return out, err
		}
		if err := c.DeleteStaticSiteAndApp(ctx, in.Name); err != nil {
			return out, fmt.Errorf("pages delete: %w", err)
		}
		out.Status = "deleted"
	default:
		return out, fmt.Errorf("pages_manage: unknown action %q, want create, deploy, domain_add, rollback or delete", in.Action)
	}
	return out, nil
}

func pagesRollback(ctx context.Context, c *apiclient.Client, in pagesInput) (pagesOutput, error) {
	out := pagesOutput{Action: in.Action}
	if err := pagesNeedName(in.Action, in.Name); err != nil {
		return out, err
	}
	if err := requireConfirm("pages rollback", "name", in.Name, in.Confirm); err != nil {
		return out, err
	}
	target := in.Deploy
	if target == "" {
		attempts, err := c.ListDeployAttempts(ctx, in.Name)
		if err != nil {
			return out, fmt.Errorf("pages rollback: list deploys: %w", err)
		}
		sort.Slice(attempts, func(i, j int) bool { return attempts[i].StartedAt.After(attempts[j].StartedAt) })
		for _, a := range attempts {
			if a.Status == "succeeded" && !a.Live {
				target = a.ID
				break
			}
		}
		if target == "" {
			return out, fmt.Errorf("pages rollback: site %q has no earlier successful deploy", in.Name)
		}
	}
	res, err := c.RollbackToDeploy(ctx, in.Name, target, apiclient.RollbackToRequest{})
	if err != nil {
		return out, fmt.Errorf("pages rollback to %q: %w", target, err)
	}
	out.DeployID, out.Status = res.DeployID, "rolled_back_to_"+target
	return out, nil
}

// pagesStageDir returns the folder to pack: the caller's dir, or a temp dir holding the inline files.
func pagesStageDir(in pagesInput) (dir string, cleanup func(), err error) {
	cleanup = func() {}
	if (in.Dir == "") == (len(in.Files) == 0) {
		return "", cleanup, errors.New("pages deploy: pass exactly one of dir or files")
	}
	if in.Dir != "" {
		st, serr := os.Stat(in.Dir)
		if serr != nil || !st.IsDir() {
			return "", cleanup, fmt.Errorf("pages deploy: dir %q is not a readable folder", in.Dir)
		}
		return in.Dir, cleanup, nil
	}
	limit := pagesEnvInt(envPagesInlineMax, defaultPagesInlineMax)
	var total int64
	for p, body := range in.Files {
		clean := path.Clean(p)
		if clean == "." || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(p, "\\") {
			return "", cleanup, fmt.Errorf("pages deploy: file path %q must be relative and stay inside the site", p)
		}
		total += int64(len(body))
	}
	if total > limit {
		return "", cleanup, fmt.Errorf("pages deploy: inline files are %d bytes, over the %d byte limit (%s); pass dir instead", total, limit, envPagesInlineMax)
	}
	tmp, err := os.MkdirTemp("", "pages-inline-*")
	if err != nil {
		return "", cleanup, fmt.Errorf("pages deploy: stage files: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(tmp) }
	for p, body := range in.Files {
		dst := filepath.Join(tmp, filepath.FromSlash(path.Clean(p)))
		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			cleanup()
			return "", func() {}, fmt.Errorf("pages deploy: stage %q: %w", p, err)
		}
		if err := os.WriteFile(dst, []byte(body), 0o600); err != nil {
			cleanup()
			return "", func() {}, fmt.Errorf("pages deploy: stage %q: %w", p, err)
		}
	}
	return tmp, cleanup, nil
}

func pagesDeploy(ctx context.Context, c *apiclient.Client, in pagesInput) (pagesOutput, error) {
	out := pagesOutput{Action: in.Action}
	name := strings.TrimSpace(in.Name)
	if name == "" && in.Dir != "" {
		name = strings.ToLower(strings.Trim(filepath.Base(filepath.Clean(in.Dir)), "."))
	}
	if name == "" {
		return out, errors.New("pages deploy: name is required")
	}
	dir, cleanup, err := pagesStageDir(in)
	if err != nil {
		return out, err
	}
	defer cleanup()
	maxBytes := pagesEnvInt(envPagesUploadMax, defaultPagesUploadMax)
	plan, err := sourceupload.Scan(ctx, dir, sourceupload.Options{MaxBytes: maxBytes})
	if err != nil {
		return out, fmt.Errorf("pages deploy: scan %s (limit %s): %w", dir, envPagesUploadMax, err)
	}
	if len(plan.Files) == 0 {
		return out, errors.New("pages deploy: nothing to publish, every file is ignored")
	}
	out.Files = len(plan.Files)

	if _, err := c.GetStaticSite(ctx, name); err != nil {
		var apiErr *apiclient.APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
			return out, fmt.Errorf("pages deploy: look up site %q: %w", name, err)
		}
		if _, err := c.CreateStaticSite(ctx, name, nil); err != nil {
			return out, fmt.Errorf("pages deploy: create site %q: %w", name, err)
		}
	}
	tmp, err := os.CreateTemp("", "pages-upload-*.tar.gz")
	if err != nil {
		return out, fmt.Errorf("pages deploy: temp archive: %w", err)
	}
	defer func() { _ = tmp.Close(); _ = os.Remove(tmp.Name()) }()
	if err := plan.WriteArchive(ctx, tmp); err != nil {
		return out, fmt.Errorf("pages deploy: pack: %w", err)
	}
	size, err := tmp.Seek(0, io.SeekCurrent)
	if err != nil {
		return out, fmt.Errorf("pages deploy: measure archive: %w", err)
	}
	if size > maxBytes {
		return out, fmt.Errorf("pages deploy: archive is %d bytes, over the %d byte limit (%s)", size, maxBytes, envPagesUploadMax)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return out, fmt.Errorf("pages deploy: rewind archive: %w", err)
	}
	up, err := c.UploadSource(ctx, name, tmp, size, nil)
	if err != nil {
		return out, fmt.Errorf("pages deploy: upload: %w", err)
	}
	trig, err := c.TriggerBuild(ctx, name, apiclient.BuildTriggerRequest{
		UploadHash: up.Hash, ImageRepo: name, Build: apiclient.BuildTriggerRequestBuild{Type: spec.BuildStatic},
	})
	if err != nil {
		return out, fmt.Errorf("pages deploy: start publish: %w", err)
	}
	out.DeployID = trig.ID
	window := time.Duration(pagesEnvInt(envPagesWaitSecs, defaultPagesWaitSecs)) * time.Second
	res, err := c.WaitForDeploy(ctx, name, trig.ID, apiclient.WaitOptions{Interval: 2 * time.Second, Window: window})
	if err != nil {
		return out, fmt.Errorf("pages deploy: deploy %s keeps running, check with the pages tool: %w", trig.ID, err)
	}
	out.Status = res.Deploy.Outcome
	if !res.Done {
		out.Status, out.Note = "timeout", "still publishing; call pages show to follow it"
		return out, nil
	}
	if res.Deploy.Outcome != apiclient.DeployOutcomeHealthy {
		out.Note = res.Deploy.Error
		return out, nil
	}
	if site, err := c.GetStaticSite(ctx, name); err == nil {
		if len(site.Domains) > 0 {
			out.URL = "https://" + site.Domains[0]
		}
	}
	return out, nil
}
