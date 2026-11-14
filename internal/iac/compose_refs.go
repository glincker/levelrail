package iac

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync/atomic"

	"gopkg.in/yaml.v3"
)

// ComposeLimits are the live gate and caps for compose references.
type ComposeLimits struct {
	Enabled       bool
	MaxFileBytes  int
	MaxFiles      int
	MaxTotalBytes int
}

var composeLive atomic.Pointer[func() ComposeLimits]

// SetComposeSource makes the gate and caps follow runtime settings.
func SetComposeSource(fn func() ComposeLimits) { composeLive.Store(&fn) }

// composeLimits is off with the documented caps until a source is set.
func composeLimits() ComposeLimits {
	if p := composeLive.Load(); p != nil {
		return (*p)()
	}
	return ComposeLimits{MaxFileBytes: 256 << 10, MaxFiles: 20, MaxTotalBytes: 2048 << 10}
}

// CleanComposePath validates a compose path and returns its cleaned form.
// It never touches the disk; callers refuse symlinks where they read files.
func CleanComposePath(p string) (string, error) {
	switch {
	case p == "":
		return "", fmt.Errorf("the compose path is empty")
	case len(p) > 512:
		return "", fmt.Errorf("the compose path is longer than 512 bytes")
	case strings.ContainsAny(p, "\x00\\"):
		return "", fmt.Errorf("the compose path holds a backslash or a NUL byte")
	case strings.HasPrefix(p, "/") || (len(p) > 1 && p[1] == ':'):
		return "", fmt.Errorf("the compose path must be relative to the folder you apply")
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "", fmt.Errorf("the compose path must not climb with parent segments")
		}
	}
	c := path.Clean(p)
	if c == "." || (!strings.HasSuffix(c, ".yaml") && !strings.HasSuffix(c, ".yml")) {
		return "", fmt.Errorf("the compose path must name a .yaml or .yml file")
	}
	return c, nil
}

// composeRefPath reads the cleaned spec.compose.path of an App document.
func composeRefPath(d *Document) (string, bool) {
	if d.Kind != KindApp {
		return "", false
	}
	n := mapValue(mapValue(d.SpecNode, "compose"), "path")
	if n == nil || n.Kind != yaml.ScalarNode {
		return "", false
	}
	c, err := CleanComposePath(n.Value)
	return c, err == nil
}

// SplitComposeRefs separates compose files referenced by App documents from
// resource documents, so a compose file in a stack folder is never parsed as
// a document. Source names are paths relative to the document set root.
func SplitComposeRefs(files []Source) (docs []Source, attachments map[string][]byte, issues []Issue) {
	type parsed struct {
		src    Source
		issues []Issue
	}
	refs := map[string]bool{}
	all := make([]parsed, len(files))
	for i, f := range files {
		ds, is := ParseDocuments([]Source{f})
		all[i] = parsed{f, is}
		for _, d := range ds {
			if c, ok := composeRefPath(d); ok {
				refs[c] = true
			}
		}
	}
	attachments = map[string][]byte{}
	for _, p := range all {
		if refs[path.Clean(p.src.Name)] {
			attachments[path.Clean(p.src.Name)] = p.src.Data
			continue
		}
		docs = append(docs, p.src)
		issues = append(issues, p.issues...)
	}
	return docs, attachments, issues
}

// ComposeRefPaths lists the cleaned compose paths the documents reference.
func ComposeRefPaths(files []Source) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range files {
		ds, _ := ParseDocuments([]Source{f})
		for _, d := range ds {
			if c, ok := composeRefPath(d); ok && !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
	}
	sort.Strings(out)
	return out
}

// checkComposeAttachments enforces the per file, count and total caps.
func checkComposeAttachments(att map[string][]byte, lim ComposeLimits) []Issue {
	var issues []Issue
	if len(att) > lim.MaxFiles {
		issues = append(issues, Issue{Code: CodeComposeTooManyFiles, Message: fmt.Sprintf("%d compose files were sent; the cap is %d (setting iac.compose_max_files)", len(att), lim.MaxFiles)})
	}
	names := make([]string, 0, len(att))
	for n := range att {
		names = append(names, n)
	}
	sort.Strings(names)
	total := 0
	for _, n := range names {
		total += len(att[n])
		if c, err := CleanComposePath(n); err != nil || c != n {
			issues = append(issues, Issue{File: n, Code: CodeComposePathInvalid, Message: "an attachment name is not a clean relative compose path"})
		}
		if len(att[n]) > lim.MaxFileBytes {
			issues = append(issues, Issue{File: n, Code: CodeComposeTooLarge, Message: fmt.Sprintf("the compose file is %d bytes; the cap is %d (setting iac.compose_max_file_kb)", len(att[n]), lim.MaxFileBytes)})
		}
	}
	if total > lim.MaxTotalBytes {
		issues = append(issues, Issue{Code: CodeComposeTooLarge, Message: fmt.Sprintf("compose files total %d bytes; the cap is %d (setting iac.compose_max_total_kb)", total, lim.MaxTotalBytes)})
	}
	return issues
}

// composeDigest covers the raw bytes, the profiles and the names (never the
// values) of the env inputs, so a byte change is an update and a secret never enters the hash.
func composeDigest(data []byte, profiles, envNames []string) string {
	sum := sha256.Sum256(data)
	h := sha256.New()
	h.Write([]byte(hex.EncodeToString(sum[:])))
	h.Write([]byte("\x00p:" + strings.Join(profiles, ",")))
	h.Write([]byte("\x00e:" + strings.Join(envNames, ",")))
	return hex.EncodeToString(h.Sum(nil))
}

func buildComposeApp(d *Document, cn *yaml.Node, hasService bool, project, environment string, tags []string, opts Options, s *issueSink) *Resource {
	lim := composeLimits()
	if !lim.Enabled {
		s.atCode(CodeUnavailable, "spec.compose", "compose references are not enabled (setting iac.compose_refs)")
		return nil
	}
	if hasService {
		s.atCode(CodeComposeModeConflict, "spec.compose", "an App sets spec.compose or spec.service, not both")
		return nil
	}
	var cs struct {
		Path     string   `yaml:"path"`
		Profiles []string `yaml:"profiles"`
	}
	if err := cn.Decode(&cs); err != nil {
		s.at("spec.compose", "%v", err)
		return nil
	}
	clean, err := CleanComposePath(cs.Path)
	if err != nil {
		s.atCode(CodeComposePathInvalid, "spec.compose.path", "%v", err)
		return nil
	}
	data, ok := opts.Attachments[clean]
	if !ok {
		s.atCode(CodeComposeRefMissing, "spec.compose.path", "compose file %q was not sent with the request; apply from the folder that holds it with a current CLI", clean)
		return nil
	}
	if len(data) > lim.MaxFileBytes {
		s.atCode(CodeComposeTooLarge, "spec.compose.path", "compose file %q is over the size cap (setting iac.compose_max_file_kb)", clean)
		return nil
	}
	env, secrets, conns, existing, refs := appEnvAt(cn, "spec.compose.env.", s, opts)
	if len(conns) > 0 {
		s.at("spec.compose.env", "connection references are not supported in compose env; use a literal or a secretRef")
	}
	if len(s.issues) > 0 {
		return nil
	}
	profiles := sortedUnique(cs.Profiles)
	names := make([]string, 0, len(env)+len(secrets)+len(refs))
	for k := range env {
		names = append(names, "l:"+k)
	}
	for _, k := range secrets {
		names = append(names, "s:"+k)
	}
	for k := range refs {
		names = append(names, "r:"+k)
	}
	sort.Strings(names)
	digest := composeDigest(data, profiles, names)
	f := map[string]any{"compose": map[string]any{"path": clean, "profiles": anyList(profiles), "digest": digest}}
	if project != "" {
		f["project"] = project
	}
	if environment != "" {
		f["environment"] = environment
		if project == "" {
			s.at("spec.environment", "an environment needs a project")
			return nil
		}
	}
	allTags := sortedUnique(tags)
	if opts.Source != "" {
		allTags = sortedUnique(append(allTags, tagManagedPre+opts.Source))
	}
	if len(allTags) > 0 {
		f["tags"] = anyList(allTags)
	}
	if len(env) > 0 {
		f["env"] = toMap(env)
	}
	return &Resource{Kind: KindApp, Name: d.Name, Doc: d, Fields: f, SecretRefs: sortedUnique(secrets), ExistingSecrets: sortedUnique(existing), ComposeDigest: digest}
}
