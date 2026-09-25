package pipeline

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/cronexpr"
)

// Issue is one validation problem, located by dotted path and source line.
type Issue struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Message string `json:"message"`
}

func (i Issue) String() string {
	if i.Line > 0 {
		return fmt.Sprintf("line %d: %s (%s)", i.Line, i.Message, i.Path)
	}
	return fmt.Sprintf("%s (%s)", i.Message, i.Path)
}

var yamlLineRe = regexp.MustCompile(`line (\d+)`)

// Validate parses data, checks it against the JSON Schema, then runs the
// semantic checks (needs graph, conditions, step arguments, cron
// expressions). The definition is non-nil only when there are no issues.
func Validate(data []byte) (*Definition, []Issue) {
	var root yaml.Node
	if err := yaml.NewDecoder(bytes.NewReader(data)).Decode(&root); err != nil {
		line := 0
		if m := yamlLineRe.FindStringSubmatch(err.Error()); m != nil {
			line, _ = strconv.Atoi(m[1])
		}
		return nil, []Issue{{Line: line, Message: "invalid yaml: " + strings.TrimPrefix(err.Error(), "yaml: ")}}
	}
	issues, err := schemaIssues(&root)
	if err != nil {
		return nil, []Issue{{Message: err.Error()}}
	}
	if len(issues) > 0 {
		return nil, issues
	}
	def, err := Parse(data)
	if err != nil {
		return nil, []Issue{{Message: strings.TrimPrefix(err.Error(), "pipeline: ")}}
	}
	issues = semanticIssues(def, &root)
	if len(issues) > 0 {
		return nil, issues
	}
	return def, nil
}

func semanticIssues(def *Definition, root *yaml.Node) []Issue {
	var issues []Issue
	add := func(path, msg string) {
		issues = append(issues, Issue{Path: path, Line: lineFor(root, strings.Split(path, ".")), Message: msg})
	}

	if def.On.IsEmpty() {
		def.On.Manual = &ManualTrigger{}
		def.On.API = true
	}
	for i, s := range def.On.Schedule {
		if _, err := cronexpr.Parse(s); err != nil {
			add(fmt.Sprintf("on.schedule.%d", i), err.Error())
		}
	}
	if def.On.Manual != nil {
		for name, in := range def.On.Manual.Inputs {
			if len(in.Options) > 0 && in.Default != "" && !slices.Contains(in.Options, in.Default) {
				add("on.manual.inputs."+name+".default", "default is not one of options")
			}
		}
	}
	if def.Concurrency != nil {
		if _, err := Interpolate(def.Concurrency.Group, Scope{Vars: map[string]string{}}); err != nil {
			add("concurrency.group", err.Error())
		}
	}

	names := make([]string, 0, len(def.Jobs))
	for k := range def.Jobs {
		names = append(names, k)
	}
	sort.Strings(names)

	for _, key := range names {
		job := def.Jobs[key]
		jp := "jobs." + key
		if len(def.Stages) > 0 && job.Stage != "" && !slices.Contains(def.Stages, job.Stage) {
			add(jp+".stage", fmt.Sprintf("stage %q is not listed in stages", job.Stage))
		}
		for i, n := range job.Needs {
			if _, ok := def.Jobs[n]; !ok {
				add(fmt.Sprintf("%s.needs.%d", jp, i), fmt.Sprintf("needs unknown job %q", n))
			} else if n == key {
				add(fmt.Sprintf("%s.needs.%d", jp, i), "a job cannot need itself")
			}
		}
		if job.If != "" {
			if err := ValidateCondition(job.If); err != nil {
				add(jp+".if", err.Error())
			}
		}
		if _, err := job.Matrix.Expand(); err != nil {
			add(jp+".matrix", err.Error())
		}
		issues = append(issues, stepIssues(root, jp, job)...)
	}

	if cyc := findCycle(def, names); cyc != "" {
		add("jobs", "dependency cycle: "+cyc)
	}
	return issues
}

func stepIssues(root *yaml.Node, jp string, job *Job) []Issue {
	var issues []Issue
	add := func(path, msg string) {
		issues = append(issues, Issue{Path: path, Line: lineFor(root, strings.Split(path, ".")), Message: msg})
	}
	ids := map[string]bool{}
	sawContainer := false
	needsContainer := false
	for i, s := range job.Steps {
		sp := fmt.Sprintf("%s.steps.%d", jp, i)
		if s.ID != "" {
			if ids[s.ID] {
				add(sp+".id", fmt.Sprintf("duplicate step id %q", s.ID))
			}
			ids[s.ID] = true
		}
		if s.If != "" {
			if err := ValidateCondition(s.If); err != nil {
				add(sp+".if", err.Error())
			}
		}
		if s.Run != "" && s.Uses != "" {
			add(sp, "a step sets either run or uses, not both")
		}
		kind := s.Kind()
		if s.NeedsContainer() {
			sawContainer = true
			needsContainer = true
		}
		if kind == KindApproval && sawContainer {
			add(sp, "approval steps must come before any step that runs in the job container")
		}
		if msg := checkWith(s, kind); msg != "" {
			add(sp+".with", msg)
		}
	}
	if needsContainer && job.Image == "" {
		add(jp+".image", "image is required when a job has steps that run in a container")
	}
	return issues
}

var knownKinds = []string{KindRun, KindTest, KindBuild, KindDeploy, KindPromote, KindRollback, KindNotify, KindApproval, KindArtifactUpload, KindArtifactDownload}

func checkWith(s Step, kind string) string {
	if !slices.Contains(knownKinds, kind) {
		return fmt.Sprintf("unknown step kind %q (known: %s)", kind, strings.Join(knownKinds, ", "))
	}
	need := func(keys ...string) string {
		for _, k := range keys {
			if s.With[k] == "" {
				return fmt.Sprintf("%s step requires with.%s", kind, k)
			}
		}
		return ""
	}
	switch kind {
	case KindTest:
		return need("command")
	case KindPromote:
		return need("from", "to")
	case KindNotify:
		return need("message")
	case KindArtifactUpload:
		return need("name", "path")
	case KindArtifactDownload:
		return need("name")
	case KindApproval:
		if a := s.With["approvers"]; a != "" && !slices.Contains([]string{"deploy", "write", "root"}, a) {
			return "approvers must be one of deploy, write, root"
		}
		if t := s.With["timeout"]; t != "" {
			if _, err := time.ParseDuration(t); err != nil {
				return "timeout is not a valid duration"
			}
		}
	}
	return ""
}

func findCycle(def *Definition, names []string) string {
	const (
		white = iota
		grey
		black
	)
	color := map[string]int{}
	var stack []string
	var visit func(string) string
	visit = func(n string) string {
		color[n] = grey
		stack = append(stack, n)
		for _, d := range def.Jobs[n].Needs {
			if _, ok := def.Jobs[d]; !ok {
				continue
			}
			switch color[d] {
			case grey:
				i := slices.Index(stack, d)
				return strings.Join(append(slices.Clone(stack[i:]), d), " -> ")
			case white:
				if c := visit(d); c != "" {
					return c
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[n] = black
		return ""
	}
	for _, n := range names {
		if color[n] == white {
			if c := visit(n); c != "" {
				return c
			}
		}
	}
	return ""
}
