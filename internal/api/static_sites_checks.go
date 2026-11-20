package api

import (
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/staticroutes"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envPagesLargeFileKB  = "APP_PAGES_LARGE_FILE_KB"
	envPagesCheckMaxFile = "APP_PAGES_CHECK_MAX_FILES"
	defaultLargeFileKB   = 1024
	defaultCheckMaxFiles = 2000
	// maxCheckHTMLBytes bounds how much of one page is parsed for tags and links.
	maxCheckHTMLBytes = 2 << 20
	// maxBrokenListed keeps the response small on a site with many dead links.
	maxBrokenListed = 20
)

// Check statuses.
const (
	checkOK   = "ok"
	checkWarn = "warn"
	checkFail = "fail"
)

// staticSiteCheck is one finding about a published Pages site.
type staticSiteCheck struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Path   string `json:"path,omitempty"`
	// Target is the link a broken_link finding points at, so callers can join it with traffic.
	Target string `json:"target,omitempty"`
}

// staticSiteChecksResponse is the body of GET /api/v1/static-sites/{name}/checks.
type staticSiteChecksResponse struct {
	Name         string            `json:"name"`
	Score        int               `json:"score"`
	FilesScanned int               `json:"files_scanned"`
	Truncated    bool              `json:"truncated"`
	TotalBytes   int64             `json:"total_bytes"`
	Checks       []staticSiteCheck `json:"checks"`
}

var (
	titleTagRe  = regexp.MustCompile(`(?is)<title[^>]*>\s*\S`)
	metaDescRe  = regexp.MustCompile(`(?is)<meta[^>]+name\s*=\s*["']description["'][^>]*>`)
	metaViewRe  = regexp.MustCompile(`(?is)<meta[^>]+name\s*=\s*["']viewport["'][^>]*>`)
	linkAttrRe  = regexp.MustCompile(`(?is)\b(?:href|src)\s*=\s*["']([^"']+)["']`)
	rasterImgRe = regexp.MustCompile(`(?i)\.(png|jpe?g|bmp|tiff?|gif)$`)
)

// handleStaticSiteChecks handles GET /api/v1/static-sites/{name}/checks: a pure
// read of the published tree, no network and no writes.
func (rt *Router) handleStaticSiteChecks(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !rt.authorizeRefs(w, r, AbilityRead, notFoundAs(http.StatusNotFound, "static site not found"), staticSiteRef(name), appRef(name)) {
		return
	}
	getter, ok := rt.staticSites.(staticSiteGetter)
	if !ok {
		writeError(w, http.StatusNotImplemented, "static sites are not configured on this control plane")
		return
	}
	site, err := getter.GetStaticSite(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "static site is not published yet")
		return
	}
	if err != nil {
		rt.internalError(w, "api: static site checks: lookup", err, slog.String("name", name))
		return
	}
	rs, err := staticroutes.LoadSidecar(site.RootDir)
	if err != nil {
		rt.internalError(w, "api: static site checks: rules", err, slog.String("name", name))
		return
	}
	if rs == nil {
		if rs, err = staticroutes.Compile(staticroutes.Input{}, staticroutes.DefaultLimits()); err != nil {
			rt.internalError(w, "api: static site checks: empty rules", err)
			return
		}
	}
	res, err := runStaticSiteChecks(site.RootDir, rs, staticCheckLimits{
		LargeFileBytes: int64(envIntDefault(envPagesLargeFileKB, defaultLargeFileKB)) << 10,
		MaxFiles:       envIntDefault(envPagesCheckMaxFile, defaultCheckMaxFiles),
	})
	if err != nil {
		writeError(w, http.StatusGone, "the published files of this site are no longer on disk")
		return
	}
	res.Name = name
	res.Checks = append(res.Checks, rt.proxyChecks(r, name, rs)...)
	res.Score = scoreChecks(res.Checks)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, res)
}

type staticCheckLimits struct {
	LargeFileBytes int64
	MaxFiles       int
}

type staticFile struct {
	rel  string
	size int64
}

func runStaticSiteChecks(rootDir string, rs *staticroutes.Ruleset, lim staticCheckLimits) (staticSiteChecksResponse, error) {
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		return staticSiteChecksResponse{}, err
	}
	defer root.Close() //nolint:errcheck // read-only handle

	res := staticSiteChecksResponse{Checks: []staticSiteCheck{}}
	var files []staticFile
	_ = filepath.WalkDir(rootDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		if len(files) >= lim.MaxFiles {
			res.Truncated = true
			return fs.SkipAll
		}
		rel, rerr := filepath.Rel(rootDir, p)
		info, ierr := d.Info()
		if rerr != nil || ierr != nil || strings.HasSuffix(rel, staticroutes.SidecarSuffix) {
			return nil
		}
		files = append(files, staticFile{rel: filepath.ToSlash(rel), size: info.Size()})
		res.TotalBytes += info.Size()
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
	res.FilesScanned = len(files)

	has := map[string]bool{}
	for _, f := range files {
		has[f.rel] = true
	}
	add := func(c staticSiteCheck) { res.Checks = append(res.Checks, c) }

	if has["index.html"] {
		add(staticSiteCheck{ID: "index_html", Status: checkOK, Title: "Home page present", Detail: "index.html is published."})
	} else {
		add(staticSiteCheck{ID: "index_html", Status: checkFail, Title: "No home page", Detail: "index.html is missing, so the site root has nothing to serve."})
	}
	if has["404.html"] || has["404/index.html"] {
		add(staticSiteCheck{ID: "custom_404", Status: checkOK, Title: "Custom 404 page", Detail: "A 404 page is published."})
	} else {
		add(staticSiteCheck{ID: "custom_404", Status: checkWarn, Title: "No custom 404 page", Detail: "Add 404.html so visitors who hit a dead link see your design."})
	}

	checkSizes(&res, files, lim)
	checkHTML(&res, root, files, rs)
	checkSecurityHeaders(&res, root, rs)
	res.Score = scoreChecks(res.Checks)
	return res, nil
}

func checkSizes(res *staticSiteChecksResponse, files []staticFile, lim staticCheckLimits) {
	var large, bigImg []staticFile
	for _, f := range files {
		if f.size <= lim.LargeFileBytes {
			continue
		}
		if rasterImgRe.MatchString(f.rel) {
			bigImg = append(bigImg, f)
		} else {
			large = append(large, f)
		}
	}
	if len(large) == 0 {
		res.Checks = append(res.Checks, staticSiteCheck{ID: "large_files", Status: checkOK, Title: "No oversized files", Detail: "Every file is under the size limit."})
	}
	for i, f := range large {
		if i >= maxBrokenListed {
			break
		}
		res.Checks = append(res.Checks, staticSiteCheck{ID: "large_file", Status: checkWarn, Title: "Large file", Detail: "This file is " + pagesSizeLabel(f.size) + ", over the " + pagesSizeLabel(lim.LargeFileBytes) + " limit.", Path: "/" + f.rel})
	}
	if len(bigImg) == 0 {
		res.Checks = append(res.Checks, staticSiteCheck{ID: "image_formats", Status: checkOK, Title: "Images are light", Detail: "No large uncompressed-format images found."})
	}
	for i, f := range bigImg {
		if i >= maxBrokenListed {
			break
		}
		res.Checks = append(res.Checks, staticSiteCheck{ID: "large_image", Status: checkWarn, Title: "Large image in a heavy format", Detail: "This image is " + pagesSizeLabel(f.size) + ". WebP or AVIF is usually much smaller.", Path: "/" + f.rel})
	}
}

func checkHTML(res *staticSiteChecksResponse, root *os.Root, files []staticFile, rs *staticroutes.Ruleset) {
	var noTitle, noDesc, noView []string
	type ref struct{ from, target string }
	var broken []ref
	seen := map[string]bool{}
	htmlCount := 0
	for _, f := range files {
		if !strings.HasSuffix(f.rel, ".html") {
			continue
		}
		htmlCount++
		body := readCapped(root, f.rel)
		if !titleTagRe.MatchString(body) {
			noTitle = append(noTitle, "/"+f.rel)
		}
		if !metaDescRe.MatchString(body) {
			noDesc = append(noDesc, "/"+f.rel)
		}
		if !metaViewRe.MatchString(body) {
			noView = append(noView, "/"+f.rel)
		}
		for _, m := range linkAttrRe.FindAllStringSubmatch(body, -1) {
			target, ok := internalTarget(f.rel, m[1])
			if !ok || seen[f.rel+"\x00"+target] {
				continue
			}
			seen[f.rel+"\x00"+target] = true
			d := rs.Resolve(root, staticroutes.Request{Method: http.MethodGet, Path: target})
			if d.Kind == staticroutes.KindNotFound {
				broken = append(broken, ref{from: "/" + f.rel, target: target})
			}
		}
	}
	meta := func(id, title, missing string, paths []string) {
		if htmlCount == 0 {
			return
		}
		if len(paths) == 0 {
			res.Checks = append(res.Checks, staticSiteCheck{ID: id, Status: checkOK, Title: title, Detail: "Every HTML page has it."})
			return
		}
		res.Checks = append(res.Checks, staticSiteCheck{ID: id, Status: checkWarn, Title: title, Detail: missing + " on " + strconv.Itoa(len(paths)) + " page(s), for example " + paths[0] + ".", Path: paths[0]})
	}
	meta("meta_title", "Page titles", "Missing <title>", noTitle)
	meta("meta_description", "Meta descriptions", "Missing meta description", noDesc)
	meta("meta_viewport", "Viewport meta tag", "Missing viewport meta tag", noView)

	if len(broken) == 0 {
		res.Checks = append(res.Checks, staticSiteCheck{ID: "broken_links", Status: checkOK, Title: "No broken links", Detail: "Every internal link and asset reference resolves to a published file."})
		return
	}
	for i, b := range broken {
		if i >= maxBrokenListed {
			res.Checks = append(res.Checks, staticSiteCheck{ID: "broken_links_more", Status: checkFail, Title: "More broken links", Detail: strconv.Itoa(len(broken)-maxBrokenListed) + " more not listed."})
			break
		}
		res.Checks = append(res.Checks, staticSiteCheck{ID: "broken_link", Status: checkFail, Title: "Broken link", Detail: b.from + " points to " + b.target + ", which does not resolve.", Path: b.from, Target: b.target})
	}
}

func checkSecurityHeaders(res *staticSiteChecksResponse, root *os.Root, rs *staticroutes.Ruleset) {
	d := rs.Resolve(root, staticroutes.Request{Method: http.MethodGet, Path: "/"})
	have := map[string]string{}
	for _, h := range d.Headers {
		have[strings.ToLower(h.Name)] = strings.ToLower(h.Value)
	}
	if have["x-content-type-options"] == "nosniff" {
		res.Checks = append(res.Checks, staticSiteCheck{ID: "header_nosniff", Status: checkOK, Title: "X-Content-Type-Options set", Detail: "nosniff is sent on the home page."})
	} else {
		res.Checks = append(res.Checks, staticSiteCheck{ID: "header_nosniff", Status: checkWarn, Title: "X-Content-Type-Options missing", Detail: "Add X-Content-Type-Options: nosniff in _headers."})
	}
	_, xfo := have["x-frame-options"]
	if xfo || strings.Contains(have["content-security-policy"], "frame-ancestors") {
		res.Checks = append(res.Checks, staticSiteCheck{ID: "header_frame", Status: checkOK, Title: "Clickjacking protection set", Detail: "X-Frame-Options or CSP frame-ancestors is sent."})
	} else {
		res.Checks = append(res.Checks, staticSiteCheck{ID: "header_frame", Status: checkWarn, Title: "No frame protection", Detail: "Add X-Frame-Options or a CSP frame-ancestors rule in _headers."})
	}
}

// internalTarget returns the absolute site path a reference points at, or false
// for external, anchor and non-http references.
func internalTarget(fromRel, raw string) (string, bool) {
	ref := strings.TrimSpace(raw)
	low := strings.ToLower(ref)
	if ref == "" || strings.HasPrefix(ref, "#") || strings.HasPrefix(ref, "//") || strings.Contains(ref, "{{") {
		return "", false
	}
	if i := strings.IndexAny(ref, "?#"); i >= 0 {
		ref = ref[:i]
	}
	if ref == "" || (strings.Contains(low, ":") && !strings.HasPrefix(ref, "/")) {
		return "", false
	}
	if !strings.HasPrefix(ref, "/") {
		ref = path.Join("/", path.Dir(fromRel), ref)
	}
	return path.Clean(ref), true
}

func readCapped(root *os.Root, rel string) string {
	f, err := root.Open(rel)
	if err != nil {
		return ""
	}
	defer f.Close() //nolint:errcheck // read-only handle
	buf := make([]byte, maxCheckHTMLBytes)
	n, _ := f.Read(buf)
	return string(buf[:n])
}

// scoreChecks starts at 100 and subtracts per finding; a fail costs more than a warn.
func scoreChecks(cs []staticSiteCheck) int {
	score := 100
	for _, c := range cs {
		switch c.Status {
		case checkFail:
			score -= 15
		case checkWarn:
			score -= 5
		}
	}
	if score < 0 {
		return 0
	}
	return score
}

func pagesSizeLabel(n int64) string {
	if n >= 1<<20 {
		return strconv.Itoa(int(n>>20)) + " MB"
	}
	return strconv.Itoa(int(n>>10)) + " KB"
}
