package pipeline

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Fix kinds. Auto kinds can be applied by the control plane, guided kinds
// point the operator at the one place the fix lives.
const (
	FixRerun        = "rerun"
	FixAddRetries   = "add_retries"
	FixRaiseTimeout = "raise_timeout"
	FixRaiseMemory  = "raise_memory"
	FixEditPipeline = "edit_pipeline"
	FixSetSecret    = "set_secret"
	FixFreeDisk     = "free_disk"
)

// Fix is a typed suggestion for a failed job. Code names the matched
// pattern, Kind says how it is applied, Target is where a guided fix lives.
type Fix struct {
	Code   string `json:"code"`
	Kind   string `json:"kind"`
	Job    string `json:"job"`
	Target string `json:"target,omitempty"`
	Hint   string `json:"hint"`
	Match  string `json:"match"`
}

type fixPattern struct {
	code   string
	re     *regexp.Regexp
	kinds  []string
	target string
	hint   string
}

var fixPatterns = []fixPattern{
	{"transient_network", regexp.MustCompile(`(?i)ETIMEDOUT|ECONNRESET|EAI_AGAIN|TLS handshake timeout|temporary failure in name resolution|503 Service Unavailable|toomanyrequests|i/o timeout|connection reset by peer`), []string{FixRerun, FixAddRetries}, "", "Network or registry hiccup. Rerun, or add retries so it heals itself."},
	{"out_of_memory", regexp.MustCompile(`(?i)OOMKilled|exit code 137|signal: killed|JavaScript heap out of memory|Cannot allocate memory`), []string{FixRaiseMemory}, "", "The job ran out of memory. Raise its memory limit."},
	{"step_timeout", regexp.MustCompile(`(?i)context deadline exceeded|timed out after|step timed out|job timed out`), []string{FixRaiseTimeout}, "", "The job hit its time limit. Raise the timeout."},
	{"disk_full", regexp.MustCompile(`(?i)no space left on device|ENOSPC`), []string{FixFreeDisk}, "node_disk", "The node is out of disk. Free space on the node, then rerun."},
	{"missing_secret", regexp.MustCompile(`(?i)secret [^\n]* not found|missing required secret|unauthorized: authentication required|denied: requested access to the resource is denied|401 Unauthorized|403 Forbidden`), []string{FixSetSecret}, "app_secrets", "A credential is missing or wrong. Set the secret the step reads."},
	{"image_not_found", regexp.MustCompile(`(?i)pull access denied|manifest unknown|manifest for [^\n]* not found|repository does not exist`), []string{FixEditPipeline}, "pipeline_editor", "The job image does not exist or is private. Fix the image name or tag."},
	{"command_not_found", regexp.MustCompile(`(?i)command not found|executable file not found|: not found$|No such file or directory: '?(npm|node|go|python|pip)`), []string{FixEditPipeline}, "pipeline_editor", "A tool is missing from the job image. Pick an image that ships it."},
	{"node_peer_deps", regexp.MustCompile(`(?i)npm ERR! code ERESOLVE|ERESOLVE unable to resolve dependency tree`), []string{FixEditPipeline}, "pipeline_editor", "Peer dependency conflict. Fix the versions, or install with --legacy-peer-deps."},
	{"node_module_missing", regexp.MustCompile(`(?i)Cannot find module|ERR_MODULE_NOT_FOUND`), []string{FixEditPipeline}, "pipeline_editor", "A dependency is not installed. Run the install step before this one."},
	{"python_module_missing", regexp.MustCompile(`(?i)ModuleNotFoundError|No module named`), []string{FixEditPipeline}, "pipeline_editor", "A Python package is not installed. Add it to requirements.txt and install first."},
	{"go_version", regexp.MustCompile(`(?i)go\.mod requires go >= [0-9.]+|requires go [0-9.]+ \(running go`), []string{FixEditPipeline}, "pipeline_editor", "The Go image is older than go.mod asks for. Use a newer golang image."},
	{"permission_denied", regexp.MustCompile(`(?i)permission denied|EACCES`), []string{FixEditPipeline}, "pipeline_editor", "A step cannot read or write a path. Check file modes and the working directory."},
}

// SuggestFixes matches the tail of a failed job's output against known error
// patterns, one fix per kind per pattern, strongest pattern first.
func SuggestFixes(job, output string) []Fix {
	lines := strings.Split(output, "\n")
	if len(lines) > 400 {
		lines = lines[len(lines)-400:]
	}
	var out []Fix
	for _, p := range fixPatterns {
		hit := ""
		for i := len(lines) - 1; i >= 0; i-- {
			if m := p.re.FindString(lines[i]); m != "" {
				hit = strings.TrimSpace(lines[i])
				if len(hit) > 200 {
					hit = hit[:200]
				}
				break
			}
		}
		if hit == "" {
			continue
		}
		for _, k := range p.kinds {
			out = append(out, Fix{Code: p.code, Kind: k, Job: job, Target: p.target, Hint: p.hint, Match: hit})
		}
	}
	return out
}

// ApplyFix rewrites a pipeline file to apply an auto fix to one job. It keeps
// comments and ordering, and refuses kinds that are not auto-applicable.
func ApplyFix(src []byte, job, kind string) ([]byte, error) {
	switch kind {
	case FixAddRetries:
		return setJobScalar(src, job, []string{"retries"}, "2", "!!int")
	case FixRaiseTimeout:
		return setJobScalar(src, job, []string{"timeout"}, "30m", "!!str")
	case FixRaiseMemory:
		return setJobScalar(src, job, []string{"resources", "memory"}, "2Gi", "!!str")
	}
	return nil, fmt.Errorf("pipeline: fix kind %q cannot be applied automatically", kind)
}

func mapChild(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func setMapScalar(m *yaml.Node, key, value, tag string) {
	if v := mapChild(m, key); v != nil {
		v.Kind, v.Tag, v.Value, v.Style = yaml.ScalarNode, tag, value, 0
		return
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value})
}

func setJobScalar(src []byte, job string, path []string, value, tag string) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, fmt.Errorf("pipeline: parse yaml: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("pipeline: file is not a mapping")
	}
	jobs := mapChild(doc.Content[0], "jobs")
	if jobs == nil || jobs.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("pipeline: file has no jobs")
	}
	node := mapChild(jobs, job)
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("pipeline: job %q not found", job)
	}
	for _, key := range path[:len(path)-1] {
		next := mapChild(node, key)
		if next == nil {
			next = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, next)
		}
		node = next
	}
	setMapScalar(node, path[len(path)-1], value, tag)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return nil, fmt.Errorf("pipeline: encode yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("pipeline: encode yaml: %w", err)
	}
	return buf.Bytes(), nil
}

// FixEditSource is a guided fix that lives in the app's source or build settings.
const FixEditSource = "edit_source"

type buildFixSpec struct {
	kind   string
	target string
	hint   string
}

// buildFixSpecs reuses fixPatterns for deploy builds, where the fix is a retry, a setting or a source change rather than a pipeline edit.
var buildFixSpecs = map[string]buildFixSpec{
	"transient_network":     {FixRerun, "", "A network or registry hiccup interrupted the build. Retry the deploy."},
	"out_of_memory":         {FixRaiseMemory, "build_performance", "The build ran out of memory. Raise the build memory limit."},
	"disk_full":             {FixFreeDisk, "node_disk", "The build node is out of disk. Free space on the node, then retry."},
	"missing_secret":        {FixSetSecret, "app_secrets", "The build needs a credential it does not have. Set the secret or registry credential."},
	"image_not_found":       {FixEditSource, "build_settings", "The base image does not exist or is private. Fix the image name, or add registry credentials."},
	"command_not_found":     {FixEditSource, "build_settings", "A tool the build needs is missing from its image. Use a base image that ships it."},
	"node_peer_deps":        {FixEditSource, "build_settings", "Peer dependency conflict. Fix the versions, or install with --legacy-peer-deps."},
	"node_module_missing":   {FixEditSource, "build_settings", "A dependency is not installed. Run the install step before the build step."},
	"python_module_missing": {FixEditSource, "build_settings", "A Python package is not installed. Add it to requirements.txt."},
	"go_version":            {FixEditSource, "build_settings", "The Go image is older than go.mod asks for. Use a newer golang image."},
	"permission_denied":     {FixEditSource, "build_settings", "A build step cannot read or write a path. Check file modes and the working directory."},
}

// SuggestBuildFixes matches a failed deploy build's log against the pipeline error patterns and returns deploy-specific typed fixes.
func SuggestBuildFixes(step, output string) []Fix {
	var out []Fix
	for _, f := range SuggestFixes(step, output) {
		spec, ok := buildFixSpecs[f.Code]
		if !ok {
			continue
		}
		f.Kind, f.Target, f.Hint = spec.kind, spec.target, spec.hint
		out = append(out, f)
	}
	return out
}
