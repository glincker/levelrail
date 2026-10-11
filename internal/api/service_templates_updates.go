package api

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/compose"
)

// Status values reported per service by GET /api/v1/service-templates/updates.
const (
	templateUpdateUpToDate  = "up_to_date"
	templateUpdateNewer     = "newer_available"
	templateUpdateFloating  = "floating_tag"
	templateUpdateUnchecked = "unchecked"
)

const (
	// templateUpdateTagPage is how many recent tags one Docker Hub lookup reads (the Hub API's maximum page).
	templateUpdateTagPage = 100
	templateUpdateMaxTags = 5
)

type templateUpdateService struct {
	Service      string   `json:"service"`
	Image        string   `json:"image"`
	Status       string   `json:"status"`
	NewerTags    []string `json:"newer_tags,omitempty"`
	CatalogImage string   `json:"catalog_image,omitempty"`
	Note         string   `json:"note,omitempty"`
}

type templateUpdateApp struct {
	App        string                  `json:"app"`
	TemplateID string                  `json:"template_id"`
	Services   []templateUpdateService `json:"services"`
}

// handleServiceTemplateUpdates handles GET /api/v1/service-templates/updates:
// for every app installed from a catalog template, reports newer upstream
// image tags for each service. Read-only: it never redeploys or edits an app.
func (rt *Router) handleServiceTemplateUpdates(w http.ResponseWriter, r *http.Request) {
	revs := rt.composeRevisionStore()
	if revs == nil {
		writeError(w, http.StatusNotImplemented, "compose history is not available on this store")
		return
	}
	ids, err := revs.ListComposeApps(r.Context())
	if err != nil {
		rt.internalError(w, "api: template updates: list compose apps failed", err)
		return
	}
	sort.Strings(ids)
	tagCache := map[string][]string{}
	out := make([]templateUpdateApp, 0)
	for _, id := range ids {
		rev, err := revs.GetComposeRevision(r.Context(), id, 0)
		if err != nil || rev.TemplateID == "" {
			continue
		}
		tpl, ok := catalog.TemplateByID(rev.TemplateID)
		if !ok {
			continue
		}
		services, err := rt.appGroups.ListServicesByApp(r.Context(), id)
		if err != nil {
			rt.internalError(w, "api: template updates: list services failed", err, slog.String("app", id))
			return
		}
		catalogImages := templateImages(tpl)
		app := templateUpdateApp{App: id, TemplateID: tpl.ID}
		for _, s := range services {
			key := strings.TrimPrefix(s.Name, id+"-")
			item := rt.checkTemplateImage(r.Context(), s.Image, tagCache)
			item.Service = key
			if want := catalogImages[key]; want != "" && want != s.Image {
				item.CatalogImage = want
			}
			app.Services = append(app.Services, item)
		}
		sort.Slice(app.Services, func(i, j int) bool { return app.Services[i].Service < app.Services[j].Service })
		out = append(out, app)
	}
	writeJSON(w, http.StatusOK, map[string]any{"apps": out})
}

// templateImages maps service key to the image the catalog currently pins.
func templateImages(tpl catalog.Template) map[string]string {
	out := map[string]string{}
	if tpl.Manifest != nil && len(tpl.Manifest.Images) > 0 {
		for _, im := range tpl.Manifest.Images {
			out[im.Service] = im.Image
		}
		return out
	}
	if f, err := compose.Parse([]byte(tpl.Compose)); err == nil {
		for k, s := range f.Services {
			out[k] = s.Image
		}
	}
	return out
}

func (rt *Router) checkTemplateImage(ctx context.Context, image string, cache map[string][]string) templateUpdateService {
	item := templateUpdateService{Image: image, Status: templateUpdateUnchecked}
	ref, err := catalog.ParseImageRef(image)
	if err != nil {
		item.Note = "image reference could not be parsed"
		return item
	}
	switch {
	case ref.Registry != "docker.io":
		item.Note = "tag listing is supported for Docker Hub images only"
	case catalog.IsFloatingTag(ref.Tag):
		item.Status = templateUpdateFloating
		item.Note = "tag floats, so a newer build keeps the same name; use the image auto-update check"
	default:
		ns, repo, ok := strings.Cut(ref.Repository, "/")
		if !ok || strings.Contains(repo, "/") {
			item.Note = "repository path is not a Docker Hub namespace/name"
			return item
		}
		tags, ok := cache[ref.Repository]
		if !ok {
			listed, err := rt.dockerHubClient.ListTags(ctx, ns, repo, templateUpdateTagPage)
			if err != nil {
				rt.logger.Warn("api: template updates: list tags failed", slog.String("repository", ref.Repository), slog.String("error", err.Error()))
				item.Note = "could not list tags from Docker Hub"
				return item
			}
			for _, t := range listed {
				tags = append(tags, t.Name)
			}
			cache[ref.Repository] = tags
		}
		item.NewerTags = catalog.NewerTags(ref.Tag, tags, templateUpdateMaxTags)
		item.Status = templateUpdateUpToDate
		if len(item.NewerTags) > 0 {
			item.Status = templateUpdateNewer
		}
	}
	return item
}
