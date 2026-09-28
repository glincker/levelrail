package pipeline

import (
	"fmt"
	"regexp"
	"strings"
)

var exprRe = regexp.MustCompile(`\$\{\{\s*(.*?)\s*\}\}`)

// expandTemplates replaces every `uses: template/<name>` step with the
// template's steps, substituting ${{ inputs.<param> }} statically. Templates
// may not invoke other templates.
func expandTemplates(def *Definition) error {
	for name, t := range def.Templates {
		for i, s := range t.Steps {
			if strings.HasPrefix(s.Uses, TemplatePrefix) {
				return fmt.Errorf("pipeline: template %q step %d: templates cannot use other templates", name, i)
			}
		}
	}
	for key, job := range def.Jobs {
		if job == nil {
			return fmt.Errorf("pipeline: job %q is empty", key)
		}
		var out []Step
		for i, s := range job.Steps {
			if !strings.HasPrefix(s.Uses, TemplatePrefix) {
				out = append(out, s)
				continue
			}
			tname := strings.TrimPrefix(s.Uses, TemplatePrefix)
			t, ok := def.Templates[tname]
			if !ok {
				return fmt.Errorf("pipeline: job %q step %d: unknown template %q", key, i, tname)
			}
			for _, p := range t.Params {
				if _, ok := s.With[p]; !ok {
					return fmt.Errorf("pipeline: job %q step %d: template %q needs parameter %q", key, i, tname, p)
				}
			}
			for _, ts := range t.Steps {
				out = append(out, substituteInputs(ts, s.With))
			}
		}
		job.Steps = out
	}
	return nil
}

func substituteInputs(s Step, inputs map[string]string) Step {
	sub := func(v string) string {
		return exprRe.ReplaceAllStringFunc(v, func(m string) string {
			inner := strings.TrimSpace(exprRe.FindStringSubmatch(m)[1])
			if name, ok := strings.CutPrefix(inner, "inputs."); ok {
				if val, ok := inputs[name]; ok {
					return val
				}
			}
			return m
		})
	}
	s.Run = sub(s.Run)
	s.Name = sub(s.Name)
	s.If = sub(s.If)
	s.With = mapValues(s.With, sub)
	s.Env = mapValues(s.Env, sub)
	return s
}

func mapValues(m map[string]string, f func(string) string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = f(v)
	}
	return out
}
