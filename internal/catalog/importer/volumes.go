package importer

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
)

var (
	volumeNameBad = regexp.MustCompile(`[^a-z0-9_.-]+`)
	fileExts      = map[string]bool{
		".conf": true, ".cfg": true, ".ini": true, ".json": true, ".yaml": true, ".yml": true,
		".toml": true, ".xml": true, ".sh": true, ".env": true, ".properties": true, ".lua": true,
		".js": true, ".php": true, ".sql": true, ".txt": true, ".key": true, ".pem": true,
		".crt": true, ".html": true, ".htpasswd": true, ".cnf": true, ".config": true,
		".py": true, ".pub": true, ".lic": true, ".db": true, ".sock": true, ".template": true,
	}
	benignHostMounts = map[string]bool{
		"/etc/localtime": true, "/etc/timezone": true,
	}
)

type volumeSpec struct {
	source   string
	target   string
	readOnly bool
	hasBody  bool
	typ      string
}

// volumes converts a service's volumes list. Relative bind mounts of
// directories become named volumes; anything that needs host files or
// file content fails the template with a reason.
func (t *translator) volumes(raw any, named map[string]string) []string {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, it := range items {
		spec, ok := t.parseVolume(it)
		if !ok {
			continue
		}
		line, ok := t.convertVolume(spec, named)
		if !ok || seen[line] {
			continue
		}
		seen[line] = true
		out = append(out, line)
	}
	return out
}

func (t *translator) parseVolume(it any) (volumeSpec, bool) {
	switch v := it.(type) {
	case string:
		s := t.translate(v, locOther)
		parts := strings.Split(s, ":")
		switch len(parts) {
		case 1:
			return volumeSpec{target: parts[0]}, true
		case 2:
			return volumeSpec{source: parts[0], target: parts[1]}, true
		default:
			return volumeSpec{source: parts[0], target: parts[1], readOnly: parts[2] == "ro"}, true
		}
	case map[string]any:
		spec := volumeSpec{
			source:   t.translate(str(v["source"]), locOther),
			target:   t.translate(str(v["target"]), locOther),
			readOnly: v["read_only"] == true,
			typ:      str(v["type"]),
		}
		_, spec.hasBody = v["content"]
		return spec, true
	default:
		t.fail(ReasonUnsupportedVolume, "volume entry has an unsupported shape")
		return volumeSpec{}, false
	}
}

func (t *translator) convertVolume(v volumeSpec, named map[string]string) (string, bool) {
	if v.target == "" {
		t.fail(ReasonUnsupportedVolume, "volume without a target")
		return "", false
	}
	if v.target == "/etc/timezone" || v.target == "/etc/localtime" {
		t.note(NoteDroppedMount, v.source+" -> "+v.target)
		return "", false
	}
	if v.typ == "tmpfs" {
		t.note(NoteDroppedMount, "tmpfs at "+v.target)
		return "", false
	}
	if strings.Contains(v.source, "${") || strings.Contains(v.source, "$") {
		t.fail(ReasonUnsupportedVolume, "volume source "+v.source+" depends on a variable")
		return "", false
	}
	if v.hasBody {
		t.fail(ReasonBindNeedsContent, "ships file content for "+v.target)
		return "", false
	}
	switch {
	case v.source == "":
		return t.namedVolume(t.svc+"-"+sanitize(v.target), v, named), true
	case strings.HasPrefix(v.source, "/"):
		if benignHostMounts[v.source] {
			t.note(NoteDroppedMount, v.source+" -> "+v.target)
			return "", false
		}
		t.fail(ReasonHostBindMount, "binds host path "+v.source)
		return "", false
	case strings.HasPrefix(v.source, ".") || strings.HasPrefix(v.source, "~"):
		return t.relativeBind(v, named)
	default:
		return t.namedVolume(v.source, v, named), true
	}
}

func (t *translator) relativeBind(v volumeSpec, named map[string]string) (string, bool) {
	if looksLikeFile(v.source) || looksLikeFile(v.target) {
		t.fail(ReasonBindNeedsContent, "mounts file "+v.source+" at "+v.target)
		return "", false
	}
	if v.readOnly {
		t.fail(ReasonBindNeedsContent, "mounts read-only directory "+v.source+" that must hold repository files")
		return "", false
	}
	rel := strings.Trim(path.Clean(v.source), "./")
	if rel == "" {
		rel = "data"
	}
	name := t.svc + "-" + sanitize(rel)
	t.note(NoteBindToVolume, v.source+" -> volume "+name)
	return t.namedVolume(name, v, named), true
}

func (t *translator) namedVolume(name string, v volumeSpec, named map[string]string) string {
	name = strings.Trim(volumeNameBad.ReplaceAllString(strings.ToLower(name), "-"), "-.")
	if name == "" {
		name = t.svc + "-data"
	}
	named[name] = v.target
	line := name + ":" + v.target
	if v.readOnly {
		line += ":ro"
	}
	return line
}

func looksLikeFile(p string) bool {
	return fileExts[strings.ToLower(path.Ext(strings.TrimRight(p, "/")))]
}

func sanitize(p string) string {
	p = strings.Trim(strings.ToLower(p), "/.")
	return strings.Trim(volumeNameBad.ReplaceAllString(strings.NewReplacer("/", "-", "_", "-").Replace(p), "-"), "-")
}

func str(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	default:
		return fmt.Sprint(x)
	}
}

// portSpec is one parsed ports: entry.
type portSpec struct {
	host      int
	container int
	proto     string
}

func parsePort(s string) (portSpec, bool) {
	proto := ""
	if i := strings.Index(s, "/"); i >= 0 {
		proto = s[i+1:]
		s = s[:i]
	}
	parts := strings.Split(s, ":")
	switch len(parts) {
	case 1:
		c, err := strconv.Atoi(parts[0])
		return portSpec{container: c, proto: proto}, err == nil && c > 0
	case 2, 3:
		c, err1 := strconv.Atoi(parts[len(parts)-1])
		h, err2 := strconv.Atoi(parts[len(parts)-2])
		return portSpec{host: h, container: c, proto: proto}, err1 == nil && err2 == nil && c > 0
	}
	return portSpec{}, false
}

func (p portSpec) String() string {
	s := strconv.Itoa(p.container)
	if p.host > 0 {
		s = strconv.Itoa(p.host) + ":" + s
	}
	if p.proto != "" {
		s += "/" + p.proto
	}
	return s
}
