package staticroutes

import (
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"strings"
)

// StatFS is the one file system call resolution needs; *os.Root satisfies it.
type StatFS interface {
	Stat(name string) (fs.FileInfo, error)
}

// Decision kinds.
const (
	KindServe    = "serve"
	KindRedirect = "redirect"
	KindNotFound = "not_found"
)

// Request is the part of an HTTP request resolution looks at. Path is the
// decoded URL path.
type Request struct {
	Method   string
	Path     string
	RawQuery string
}

// Decision is the outcome for one request. File is relative to the site
// root. Trace lists, in order, every step considered.
type Decision struct {
	Kind     string     `json:"kind"`
	Status   int        `json:"status"`
	Location string     `json:"location,omitempty"`
	File     string     `json:"file,omitempty"`
	Rule     string     `json:"rule,omitempty"`
	Headers  []HeaderKV `json:"headers"`
	Remove   []string   `json:"remove_headers"`
	Trace    []string   `json:"trace"`
}

// reserved files hold the rules themselves and are never served.
var reserved = map[string]bool{"/" + SourceRedirects: true, "/" + SourceHeaders: true}

const maxRequestPath = 2048

// Resolve applies the documented precedence: explicit rules in order (the
// first that matches, and, unless forced, only when no file is at the path),
// then files (exact, clean URL, directory index), then the SPA fallback,
// then the 404 page.
func (rs *Ruleset) Resolve(fsys StatFS, req Request) Decision {
	d := Decision{Headers: []HeaderKV{}, Remove: []string{}, Trace: []string{}}
	p := cleanRequestPath(req.Path)
	if len(p) > maxRequestPath {
		d.Kind, d.Status = KindNotFound, 414
		d.Trace = append(d.Trace, "path too long")
		return d
	}
	rs.applyHeaders(&d, p)
	rs.resolvePath(fsys, req, p, &d)
	return d
}

func cleanRequestPath(p string) string {
	trailing := strings.HasSuffix(p, "/")
	c := path.Clean("/" + p)
	if trailing && c != "/" {
		c += "/"
	}
	return c
}

func (rs *Ruleset) resolvePath(fsys StatFS, req Request, p string, d *Decision) {
	q := parseQuery(req.RawQuery)
	fileAtPath := rs.fileAt(fsys, p)
	for i := range rs.Rules {
		r := &rs.Rules[i]
		caps := r.Pattern().Match(p)
		if caps == nil || !matchQuery(r, q, caps) {
			continue
		}
		if !r.Force && fileAtPath {
			d.Trace = append(d.Trace, fmt.Sprintf("%s matches but a file exists at %s and the rule is not forced (add ! to the status)", r.Source, p))
			continue
		}
		if rs.applyRule(fsys, req, r, caps, p, d) {
			return
		}
	}
	rs.resolveFiles(fsys, req, p, d)
}

func (rs *Ruleset) applyRule(fsys StatFS, req Request, r *Rule, caps map[string]string, p string, d *Decision) bool {
	to := substitute(r.To, caps)
	switch {
	case isRedirectStatus(r.Status):
		loc := guardLocation(withQuery(to, req.RawQuery))
		if strings.HasPrefix(to, "/") && strings.TrimSuffix(to, "/") == strings.TrimSuffix(p, "/") && !strings.Contains(to, "?") {
			d.Trace = append(d.Trace, fmt.Sprintf("%s skipped: it would redirect %s to itself", r.Source, p))
			return false
		}
		d.Kind, d.Status, d.Location, d.Rule = KindRedirect, r.Status, loc, r.Source
		d.Trace = append(d.Trace, fmt.Sprintf("%s: redirect %d to %s", r.Source, r.Status, loc))
		return true
	case r.Status == 200:
		target := cleanRequestPath(trimQuery(to))
		file, _, ok := rs.findFile(fsys, target)
		if !ok {
			d.Trace = append(d.Trace, fmt.Sprintf("%s: rewrite target %s has no file, falling through", r.Source, target))
			return false
		}
		d.Kind, d.Status, d.File, d.Rule = KindServe, 200, file, r.Source
		d.Trace = append(d.Trace, fmt.Sprintf("%s: rewrite to %s serving %s", r.Source, target, file))
		return true
	default:
		target := cleanRequestPath(trimQuery(to))
		file, _, ok := rs.findFile(fsys, target)
		d.Kind, d.Status, d.Rule = KindNotFound, r.Status, r.Source
		if ok {
			d.File = file
		}
		d.Trace = append(d.Trace, fmt.Sprintf("%s: answer %d with %s", r.Source, r.Status, target))
		return true
	}
}

func (rs *Ruleset) resolveFiles(fsys StatFS, req Request, p string, d *Decision) {
	opt := rs.Options
	if opt.CleanURLs && strings.HasSuffix(p, ".html") && !reserved[p] && p != "/404.html" && rs.fileAt(fsys, p) {
		clean := strings.TrimSuffix(p, ".html")
		if strings.HasSuffix(p, "/index.html") {
			clean = strings.TrimSuffix(p, "index.html")
		}
		d.setRedirect(301, withQuery(clean, req.RawQuery), "clean URLs: "+p+" is served at "+clean)
		return
	}
	file, viaDir, ok := rs.findFile(fsys, p)
	if ok {
		if loc, redirect := rs.slashRedirect(p, viaDir); redirect {
			d.setRedirect(308, withQuery(loc, req.RawQuery), "trailing slash: "+p+" is served at "+loc)
			return
		}
		d.Kind, d.Status, d.File = KindServe, 200, file
		d.Trace = append(d.Trace, "file "+file)
		return
	}
	if viaDir == dirNoIndex {
		d.Trace = append(d.Trace, p+" is a directory without index.html")
	}
	if opt.SPA && (req.Method == "GET" || req.Method == "HEAD" || req.Method == "") && !strings.Contains(path.Base(p), ".") {
		if idx, _, found := rs.findFile(fsys, "/index.html"); found {
			d.Kind, d.Status, d.File = KindServe, 200, idx
			d.Trace = append(d.Trace, "no file at "+p+": SPA fallback to index.html")
			return
		}
	}
	d.Kind, d.Status = KindNotFound, 404
	if page := rs.notFoundPage(fsys); page != "" {
		d.File = page
		d.Trace = append(d.Trace, "no file at "+p+": 404 page "+page)
		return
	}
	d.Trace = append(d.Trace, "no file at "+p+": plain 404")
}

func (d *Decision) setRedirect(status int, loc, why string) {
	d.Kind, d.Status, d.Location = KindRedirect, status, loc
	d.Trace = append(d.Trace, why)
}

// slashRedirect applies the trailing slash policy to a path that resolved.
func (rs *Ruleset) slashRedirect(p string, via int) (string, bool) {
	switch rs.Options.TrailingSlash {
	case SlashRemove:
		if p != "/" && strings.HasSuffix(p, "/") {
			return strings.TrimRight(p, "/"), true
		}
		return "", false
	case SlashAdd:
		if !strings.HasSuffix(p, "/") && via != exactFile {
			return p + "/", true
		}
		return "", false
	}
	if via == dirIndex && !strings.HasSuffix(p, "/") && !rs.Options.CleanURLs {
		return p + "/", true
	}
	return "", false
}

// How findFile located a file.
const (
	exactFile = iota
	cleanFile
	dirIndex
	dirNoIndex
)

// findFile maps a request path to a file under the root, never a reserved one.
func (rs *Ruleset) findFile(fsys StatFS, p string) (string, int, bool) {
	rel := strings.TrimPrefix(p, "/")
	isFile := func(name string) bool {
		if reserved["/"+name] {
			return false
		}
		fi, err := fsys.Stat(name)
		return err == nil && fi.Mode().IsRegular()
	}
	if strings.HasSuffix(p, "/") || p == "/" {
		idx := path.Join(rel, "index.html")
		if isFile(idx) {
			return idx, dirIndex, true
		}
		return "", dirNoIndex, false
	}
	if isFile(rel) {
		return rel, exactFile, true
	}
	if rs.Options.CleanURLs && isFile(rel+".html") {
		return rel + ".html", cleanFile, true
	}
	if fi, err := fsys.Stat(rel); err == nil && fi.IsDir() {
		if idx := path.Join(rel, "index.html"); isFile(idx) {
			return idx, dirIndex, true
		}
		return "", dirNoIndex, false
	}
	return "", exactFile, false
}

func (rs *Ruleset) fileAt(fsys StatFS, p string) bool {
	_, _, ok := rs.findFile(fsys, p)
	return ok
}

func (rs *Ruleset) notFoundPage(fsys StatFS) string {
	name := rs.Options.NotFound
	if name == "" {
		name = "404.html"
	}
	name = path.Clean(name)
	if reserved["/"+name] {
		return ""
	}
	if fi, err := fsys.Stat(name); err == nil && fi.Mode().IsRegular() {
		return name
	}
	return ""
}

// applyHeaders merges every header block whose path matches: values of one
// name are joined with ", " in file order, and "! Name" removes it last.
func (rs *Ruleset) applyHeaders(d *Decision, p string) {
	idx := map[string]int{}
	for i := range rs.Headers {
		b := &rs.Headers[i]
		if b.Pattern().Match(p) == nil {
			continue
		}
		for _, kv := range b.Set {
			if j, ok := idx[kv.Name]; ok {
				d.Headers[j].Value += ", " + kv.Value
				continue
			}
			idx[kv.Name] = len(d.Headers)
			d.Headers = append(d.Headers, kv)
		}
		d.Remove = append(d.Remove, b.Remove...)
		d.Trace = append(d.Trace, fmt.Sprintf("headers %s apply", b.Source))
	}
}

func parseQuery(raw string) url.Values {
	v, _ := url.ParseQuery(raw)
	return v
}

func matchQuery(r *Rule, q url.Values, caps map[string]string) bool {
	for _, m := range r.Query {
		got, ok := q[m.Key]
		if !ok || len(got) == 0 || got[0] == "" {
			return false
		}
		if strings.HasPrefix(m.Value, ":") {
			caps[m.Value[1:]] = got[0]
		} else if got[0] != m.Value {
			return false
		}
	}
	return true
}

func trimQuery(s string) string {
	before, _, _ := strings.Cut(s, "?")
	return before
}

// withQuery carries the visitor's query string over unless the target has its own.
func withQuery(to, raw string) string {
	if raw == "" || strings.Contains(to, "?") {
		return to
	}
	return to + "?" + raw
}
