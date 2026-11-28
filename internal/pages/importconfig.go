// Package pages converts existing static hosting config files into the
// _redirects and _headers formats. It is pure local text conversion.
package pages

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/glincker/levelrail/internal/spec"
)

// maxImportFile bounds how much of any candidate file is read.
const maxImportFile = 1 << 20

// HeaderBlock is one path pattern with its response headers.
type HeaderBlock struct {
	Path    string
	Headers [][2]string
}

// ImportResult is the converted config plus a report of what was skipped.
type ImportResult struct {
	Redirects   []string
	Headers     []HeaderBlock
	Unsupported []string
	Sources     []string
}

// ImportDir inspects the root of dir, detects config files by shape and
// converts them. Nothing is written.
func ImportDir(dir string) (*ImportResult, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	res := &ImportResult{}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Type().IsRegular() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		if err := importOne(dir, name, res); err != nil {
			return nil, err
		}
	}
	return res, nil
}

func importOne(dir, name string, res *ImportResult) error {
	lower := strings.ToLower(name)
	isPlain := name == "_redirects" || name == "_headers"
	isToml := strings.HasSuffix(lower, ".toml")
	isJSON := strings.HasSuffix(lower, ".json")
	if !isPlain && !isToml && !isJSON {
		return nil
	}
	data, err := readCapped(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	text := string(data)
	switch {
	case name == "_redirects":
		res.Sources = append(res.Sources, name+" (plain redirects)")
		importPlainRedirects(name, text, res)
	case name == "_headers":
		res.Sources = append(res.Sources, name+" (plain headers)")
		importPlainHeaders(text, res)
	case isToml:
		if strings.Contains(text, "[[redirects]]") || strings.Contains(text, "[[headers]]") {
			res.Sources = append(res.Sources, name+" (toml rules)")
			importTomlRules(name, text, res)
		}
	case isJSON:
		if ok := importJSONRules(name, data, res); ok {
			res.Sources = append(res.Sources, name+" (json rules)")
		}
	}
	return nil
}

func readCapped(path string) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // path comes from a user-chosen dir listing
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, maxImportFile)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return nil, nil
	}
	return buf[:n], nil
}

func (r *ImportResult) skip(src, why string) {
	r.Unsupported = append(r.Unsupported, fmt.Sprintf("%s: %s", src, why))
}

func (r *ImportResult) addRedirect(from, to string, status int) {
	if err := checkRedirectForms(from, to); err != nil {
		r.skip(from+" -> "+to, err.Error())
		return
	}
	line := from + " " + to
	if status != 0 {
		line += fmt.Sprintf(" %d", status)
	}
	for _, e := range r.Redirects {
		if e == line {
			return
		}
	}
	r.Redirects = append(r.Redirects, line)
}

func (r *ImportResult) addHeader(path, key, value string) {
	if err := checkPathForms(path); err != nil {
		r.skip("header path "+path, err.Error())
		return
	}
	for i := range r.Headers {
		if r.Headers[i].Path == path {
			r.Headers[i].Headers = append(r.Headers[i].Headers, [2]string{key, value})
			return
		}
	}
	r.Headers = append(r.Headers, HeaderBlock{Path: path, Headers: [][2]string{{key, value}}})
}

func importPlainRedirects(src, text string, res *ImportResult) {
	for i, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 2 {
			res.skip(fmt.Sprintf("%s line %d", src, i+1), "needs a source and a destination")
			continue
		}
		status := 0
		var extra []string
		for _, tok := range f[2:] {
			if s, ok := parseStatus(tok); ok && status == 0 {
				status = s
			} else {
				extra = append(extra, tok)
			}
		}
		if len(extra) > 0 {
			res.skip(fmt.Sprintf("%s line %d", src, i+1), "conditions or query matchers are not supported: "+strings.Join(extra, " "))
			continue
		}
		res.addRedirect(f[0], f[1], status)
	}
}

// parseStatus accepts "301" and the forced form "301!".
func parseStatus(tok string) (int, bool) {
	tok = strings.TrimSuffix(tok, "!")
	if len(tok) != 3 {
		return 0, false
	}
	n := 0
	for _, c := range tok {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, n >= 200 && n < 600
}

func importPlainHeaders(text string, res *ImportResult) {
	cur := ""
	for _, raw := range strings.Split(text, "\n") {
		if strings.TrimSpace(raw) == "" || strings.HasPrefix(strings.TrimSpace(raw), "#") {
			continue
		}
		if raw[0] != ' ' && raw[0] != '\t' {
			cur = strings.TrimSpace(raw)
			continue
		}
		k, v, ok := strings.Cut(strings.TrimSpace(raw), ":")
		if !ok || cur == "" {
			continue
		}
		if strings.HasPrefix(k, "!") {
			res.skip("_headers "+cur, "header removal is not supported: "+k)
			continue
		}
		res.addHeader(cur, strings.TrimSpace(k), strings.TrimSpace(v))
	}
}

// Render returns the _redirects and _headers file bodies.
func (r *ImportResult) Render() (redirects, headers string) {
	var rb strings.Builder
	for _, l := range r.Redirects {
		rb.WriteString(l + "\n")
	}
	var hb strings.Builder
	for _, b := range r.Headers {
		hb.WriteString(b.Path + "\n")
		for _, h := range b.Headers {
			hb.WriteString("  " + h[0] + ": " + h[1] + "\n")
		}
	}
	return rb.String(), hb.String()
}

// checkPathForms rejects wildcard shapes the engine does not match: regex
// groups, :name+ or :name?, and a * that is not a whole last segment.
func checkPathForms(p string) error {
	for _, seg := range strings.Split(strings.Trim(p, "/"), "/") {
		if strings.ContainsAny(seg, "()+?[]{}|\\") || seg != "*" && strings.Contains(seg, "*") {
			return fmt.Errorf("pattern form %q is not supported (use :name or a trailing /*)", seg)
		}
	}
	if _, err := spec.CompileStaticPattern(p); err != nil {
		return err
	}
	return nil
}

// checkRedirectForms makes sure every :name in to is captured by from and
// that :splat only appears when from ends in /*, as the engine requires.
func checkRedirectForms(from, to string) error {
	if err := checkPathForms(from); err != nil {
		return err
	}
	known := map[string]bool{}
	for _, seg := range strings.Split(from, "/") {
		if strings.HasPrefix(seg, ":") {
			known[seg[1:]] = true
		}
	}
	hasSplat := from == "/*" || strings.HasSuffix(from, "/*")
	if hasSplat {
		known["splat"] = true
	}
	target := to
	if i := strings.Index(target, "://"); i >= 0 {
		target = target[i+3:]
	}
	for _, m := range spec.StaticPlaceholderRe.FindAllStringSubmatch(target, -1) {
		if !known[m[1]] {
			return fmt.Errorf("destination uses :%s which the source does not capture", m[1])
		}
	}
	return nil
}
