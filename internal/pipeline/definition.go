// Package pipeline is a user-defined CI/CD layer on top of the existing
// build and deploy primitives. A pipeline is a YAML file describing
// triggers, jobs, and steps; the Engine drives runs level-triggered from
// persisted state and never touches the reconciler directly.
package pipeline

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// SchemaVersion is the only pipeline file version this package accepts.
const SchemaVersion = 1

// Step kinds, the values of a step's `uses` field. A step with a `run`
// script is kind "run".
const (
	KindRun              = "run"
	KindTest             = "test"
	KindBuild            = "build"
	KindDeploy           = "deploy"
	KindPromote          = "promote"
	KindRollback         = "rollback"
	KindNotify           = "notify"
	KindApproval         = "approval"
	KindArtifactUpload   = "artifact-upload"
	KindArtifactDownload = "artifact-download"
	TemplatePrefix       = "template/"
)

// Trigger kinds recorded on a run.
const (
	TriggerPush        = "push"
	TriggerPullRequest = "pull_request"
	TriggerTag         = "tag"
	TriggerManual      = "manual"
	TriggerSchedule    = "schedule"
	TriggerAPI         = "api"
)

// Duration is a time.Duration that unmarshals from a Go duration string.
type Duration time.Duration

// UnmarshalYAML parses values like "30s" or "1h30m".
func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	var s string
	if err := n.Decode(&s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q", s)
	}
	*d = Duration(v)
	return nil
}

// MarshalYAML renders the duration in Go duration syntax.
func (d Duration) MarshalYAML() (any, error) { return time.Duration(d).String(), nil }

// StringList unmarshals either a single string or a list of strings.
type StringList []string

// UnmarshalYAML accepts a scalar or a sequence.
func (l *StringList) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		var s string
		if err := n.Decode(&s); err != nil {
			return err
		}
		*l = StringList{s}
		return nil
	}
	var out []string
	if err := n.Decode(&out); err != nil {
		return err
	}
	*l = out
	return nil
}

// Definition is a parsed pipeline file.
type Definition struct {
	Version     int                 `yaml:"version" json:"version"`
	Name        string              `yaml:"name" json:"name"`
	On          Triggers            `yaml:"on" json:"on"`
	Stages      []string            `yaml:"stages,omitempty" json:"stages,omitempty"`
	Env         map[string]string   `yaml:"env,omitempty" json:"env,omitempty"`
	Concurrency *Concurrency        `yaml:"concurrency,omitempty" json:"concurrency,omitempty"`
	Timeout     Duration            `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Templates   map[string]Template `yaml:"templates,omitempty" json:"templates,omitempty"`
	Jobs        map[string]*Job     `yaml:"jobs" json:"jobs"`
	// JobOrder is the order jobs appear in the file, kept for stable display.
	JobOrder []string `yaml:"-" json:"job_order"`
}

// Triggers lists what starts a run. A nil field means that trigger is off.
type Triggers struct {
	Push        *RefTrigger    `yaml:"push,omitempty" json:"push,omitempty"`
	PullRequest *PRTrigger     `yaml:"pull_request,omitempty" json:"pull_request,omitempty"`
	Tag         *TagTrigger    `yaml:"tag,omitempty" json:"tag,omitempty"`
	Manual      *ManualTrigger `yaml:"manual,omitempty" json:"manual,omitempty"`
	Schedule    []string       `yaml:"schedule,omitempty" json:"schedule,omitempty"`
	API         bool           `yaml:"api,omitempty" json:"api,omitempty"`
}

// RefTrigger filters push and pull request events by branch glob.
type RefTrigger struct {
	Branches StringList `yaml:"branches,omitempty" json:"branches,omitempty"`
}

// Fork policies for pull requests whose source is another repository.
const (
	ForksBlock   = "block"
	ForksApprove = "approve"
	ForksAllow   = "allow"
)

// PRTrigger filters pull request events by target branch glob and sets
// what happens to a pull request from a fork.
type PRTrigger struct {
	Branches StringList `yaml:"branches,omitempty" json:"branches,omitempty"`
	// Forks is block (the default), approve (hold the run until an
	// approver releases it), or allow.
	Forks string `yaml:"forks,omitempty" json:"forks,omitempty"`
}

// ForkPolicy returns the effective fork policy, defaulting to block.
func (t *PRTrigger) ForkPolicy() string {
	if t == nil || t.Forks == "" {
		return ForksBlock
	}
	return t.Forks
}

// TagTrigger filters tag pushes by glob.
type TagTrigger struct {
	Patterns StringList `yaml:"patterns,omitempty" json:"patterns,omitempty"`
}

// ManualTrigger declares the inputs a manual run prompts for.
type ManualTrigger struct {
	Inputs map[string]Input `yaml:"inputs,omitempty" json:"inputs,omitempty"`
}

// Input is one manual-run parameter.
type Input struct {
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	Default     string   `yaml:"default,omitempty" json:"default,omitempty"`
	Required    bool     `yaml:"required,omitempty" json:"required,omitempty"`
	Options     []string `yaml:"options,omitempty" json:"options,omitempty"`
}

// Concurrency serializes runs sharing a group; CancelInProgress cancels the
// older active runs in the group instead of queueing behind them.
type Concurrency struct {
	Group            string `yaml:"group" json:"group"`
	CancelInProgress bool   `yaml:"cancel_in_progress,omitempty" json:"cancel_in_progress,omitempty"`
}

// Template is a reusable list of steps invoked with `uses: template/<name>`.
// Parameters are referenced as ${{ inputs.<param> }} inside the steps.
type Template struct {
	Params []string `yaml:"params,omitempty" json:"params,omitempty"`
	Steps  []Step   `yaml:"steps" json:"steps"`
}

// Matrix expands a job into one instance per combination of Vars.
type Matrix struct {
	Vars    map[string]StringList `yaml:",inline" json:"vars,omitempty"`
	Include []map[string]string   `yaml:"include,omitempty" json:"include,omitempty"`
	Exclude []map[string]string   `yaml:"exclude,omitempty" json:"exclude,omitempty"`
}

// Resources caps a job container. Memory uses Kubernetes-style suffixes.
type Resources struct {
	Memory string  `yaml:"memory,omitempty" json:"memory,omitempty"`
	CPU    float64 `yaml:"cpu,omitempty" json:"cpu,omitempty"`
}

// CacheSpec mounts a persistent named volume at Path. Runs sharing a Key
// share the volume.
type CacheSpec struct {
	Key  string `yaml:"key" json:"key"`
	Path string `yaml:"path" json:"path"`
}

// Job is a group of steps run in one container on one node.
type Job struct {
	Name            string            `yaml:"name,omitempty" json:"name,omitempty"`
	Stage           string            `yaml:"stage,omitempty" json:"stage,omitempty"`
	Needs           StringList        `yaml:"needs,omitempty" json:"needs,omitempty"`
	If              string            `yaml:"if,omitempty" json:"if,omitempty"`
	Image           string            `yaml:"image,omitempty" json:"image,omitempty"`
	Node            string            `yaml:"node,omitempty" json:"node,omitempty"`
	Env             map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	Matrix          *Matrix           `yaml:"matrix,omitempty" json:"matrix,omitempty"`
	Resources       *Resources        `yaml:"resources,omitempty" json:"resources,omitempty"`
	Timeout         Duration          `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Retries         int               `yaml:"retries,omitempty" json:"retries,omitempty"`
	ContinueOnError bool              `yaml:"continue_on_error,omitempty" json:"continue_on_error,omitempty"`
	Checkout        *bool             `yaml:"checkout,omitempty" json:"checkout,omitempty"`
	Cache           []CacheSpec       `yaml:"cache,omitempty" json:"cache,omitempty"`
	Steps           []Step            `yaml:"steps" json:"steps"`
}

// Step is one unit of work. Exactly one of Run or Uses is set.
type Step struct {
	ID              string            `yaml:"id,omitempty" json:"id,omitempty"`
	Name            string            `yaml:"name,omitempty" json:"name,omitempty"`
	Run             string            `yaml:"run,omitempty" json:"run,omitempty"`
	Uses            string            `yaml:"uses,omitempty" json:"uses,omitempty"`
	With            map[string]string `yaml:"with,omitempty" json:"with,omitempty"`
	Env             map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	Secrets         []string          `yaml:"secrets,omitempty" json:"secrets,omitempty"`
	If              string            `yaml:"if,omitempty" json:"if,omitempty"`
	Timeout         Duration          `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Retries         int               `yaml:"retries,omitempty" json:"retries,omitempty"`
	ContinueOnError bool              `yaml:"continue_on_error,omitempty" json:"continue_on_error,omitempty"`
}

// Kind returns the step's kind, KindRun for a script step.
func (s Step) Kind() string {
	if s.Run != "" || s.Uses == "" {
		return KindRun
	}
	return s.Uses
}

// DisplayName is the step's name, falling back to its kind or script.
func (s Step) DisplayName() string {
	if s.Name != "" {
		return s.Name
	}
	if s.Run != "" {
		return firstLine(s.Run, 60)
	}
	return s.Uses
}

func firstLine(s string, limit int) string {
	for i, r := range s {
		if r == '\n' {
			s = s[:i]
			break
		}
	}
	if len(s) > limit {
		return s[:limit] + "..."
	}
	return s
}

// NeedsContainer reports whether the step executes inside the job container.
func (s Step) NeedsContainer() bool {
	switch s.Kind() {
	case KindRun, KindTest, KindArtifactUpload, KindArtifactDownload:
		return true
	}
	return false
}

// Parse decodes YAML into a Definition and expands step templates. It does
// not validate; call Validate for structural and semantic checks.
func Parse(data []byte) (*Definition, error) {
	var def Definition
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&def); err != nil {
		return nil, fmt.Errorf("pipeline: parse yaml: %w", err)
	}
	var order struct {
		Jobs yaml.Node `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &order); err == nil {
		for i := 0; i+1 < len(order.Jobs.Content); i += 2 {
			def.JobOrder = append(def.JobOrder, order.Jobs.Content[i].Value)
		}
	}
	if len(def.Jobs) == 0 {
		return nil, errors.New("pipeline: no jobs defined")
	}
	if err := expandTemplates(&def); err != nil {
		return nil, err
	}
	return &def, nil
}
