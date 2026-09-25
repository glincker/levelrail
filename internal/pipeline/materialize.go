package pipeline

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// KindSetup is the implicit step that prepares the workspace and container.
const KindSetup = "setup"

// plannedStep maps a stored step row to a definition step (Def is -1 for
// the implicit setup step).
type plannedStep struct {
	Kind string
	Name string
	Def  int
}

// planSteps lists a job's stored step rows: the definition steps in order,
// with a setup step inserted before the first step that needs a container.
func planSteps(j *Job) []plannedStep {
	var out []plannedStep
	setup := false
	for i, s := range j.Steps {
		if s.NeedsContainer() && !setup {
			out = append(out, plannedStep{Kind: KindSetup, Name: "Set up job", Def: -1})
			setup = true
		}
		out = append(out, plannedStep{Kind: s.Kind(), Name: s.DisplayName(), Def: i})
	}
	return out
}

// materialize expands the definition into job and step rows for a run.
func (e *Engine) materialize(run store.PipelineRun, def *Definition) ([]store.PipelineJob, error) {
	var rows []store.PipelineJob
	order := def.JobOrder
	if len(order) != len(def.Jobs) {
		order = sortedJobNames(def)
	}
	for _, name := range order {
		jd := def.Jobs[name]
		combos, err := jd.Matrix.Expand()
		if err != nil {
			return nil, fmt.Errorf("job %q: %w", name, err)
		}
		needs := implicitNeeds(def, name, jd)
		needsJSON, _ := json.Marshal(needs)
		for _, c := range combos {
			matrixJSON, _ := json.Marshal(c)
			row := store.PipelineJob{
				ID: e.cfg.NewID(), RunID: run.ID, Key: ComboKey(name, c), DisplayName: firstNonEmpty(jd.Name, name),
				Stage: jd.Stage, NeedsJSON: string(needsJSON), MatrixJSON: string(matrixJSON), NodeID: jd.Node,
				Status: store.PipelineStatusPending,
			}
			for i, p := range planSteps(jd) {
				row.Steps = append(row.Steps, store.PipelineStep{Index: i, Name: p.Name, Kind: p.Kind})
			}
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func sortedJobNames(def *Definition) []string {
	names := make([]string, 0, len(def.Jobs))
	for k := range def.Jobs {
		names = append(names, k)
	}
	slices.Sort(names)
	return names
}

// implicitNeeds adds the sequencing that `stages` imply: a job waits for
// every job in an earlier stage.
func implicitNeeds(def *Definition, name string, jd *Job) []string {
	needs := slices.Clone([]string(jd.Needs))
	si := slices.Index(def.Stages, jd.Stage)
	if jd.Stage == "" || si <= 0 {
		return needs
	}
	for other, oj := range def.Jobs {
		if other == name {
			continue
		}
		if oi := slices.Index(def.Stages, oj.Stage); oj.Stage != "" && oi >= 0 && oi < si && !slices.Contains(needs, other) {
			needs = append(needs, other)
		}
	}
	slices.Sort(needs)
	return needs
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// runVars are the scope values that depend only on the run.
func runVars(run store.PipelineRun, def *Definition) map[string]string {
	v := map[string]string{
		"app": run.AppName, "trigger": run.TriggerKind, "actor": run.TriggerActor, "ref": run.Ref, "sha": run.CommitSHA,
		"run.number": fmt.Sprint(run.Number), "run.id": run.ID, "pipeline": def.Name,
	}
	v["branch"] = strings.TrimPrefix(run.Ref, "refs/heads/")
	if strings.HasPrefix(run.Ref, "refs/tags/") {
		v["tag"] = strings.TrimPrefix(run.Ref, "refs/tags/")
		v["branch"] = ""
	}
	var inputs map[string]string
	if json.Unmarshal([]byte(run.InputsJSON), &inputs) == nil {
		for k, val := range inputs {
			v["inputs."+k] = val
		}
	}
	for k, val := range def.Env {
		v["env."+k] = val
	}
	return v
}

// jobScope builds the scope for evaluating a job's condition and running
// its steps, including needs results and outputs.
func jobScope(run store.PipelineRun, def *Definition, row store.PipelineJob, all []store.PipelineJob) Scope {
	vars := runVars(run, def)
	vars["job"] = row.Key
	var combo map[string]string
	if json.Unmarshal([]byte(row.MatrixJSON), &combo) == nil {
		for k, val := range combo {
			vars["matrix."+k] = val
		}
	}
	if jd := def.Jobs[baseJobName(row.Key)]; jd != nil {
		for k, val := range jd.Env {
			vars["env."+k] = val
		}
	}
	var needs []string
	_ = json.Unmarshal([]byte(row.NeedsJSON), &needs)
	success, failure := true, false
	for _, n := range needs {
		res := "success"
		for _, o := range all {
			if baseJobName(o.Key) != n {
				continue
			}
			ok := o.Status == store.PipelineStatusSucceeded || (o.Status == store.PipelineStatusFailed && def.Jobs[n] != nil && def.Jobs[n].ContinueOnError)
			if !ok {
				success = false
				res = resultOf(o.Status)
			}
			if o.Status == store.PipelineStatusFailed && !ok {
				failure = true
			}
			var outs map[string]string
			if json.Unmarshal([]byte(o.OutputsJSON), &outs) == nil {
				for k, val := range outs {
					vars["needs."+n+".outputs."+k] = val
				}
			}
		}
		vars["needs."+n+".result"] = res
	}
	return Scope{Vars: vars, Success: success, Failure: failure}
}

func resultOf(status string) string {
	switch status {
	case store.PipelineStatusSucceeded:
		return "success"
	case store.PipelineStatusFailed:
		return "failure"
	}
	return status
}
