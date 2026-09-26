package api

import "github.com/GLINCKER/levelrail/internal/diagnose"

type diagnosisChangeResource struct {
	Field      string `json:"field"`
	From       string `json:"from"`
	To         string `json:"to"`
	NeedsInput bool   `json:"needs_input,omitempty"`
}

type diagnosisFixResource struct {
	N        int                       `json:"n"`
	Label    string                    `json:"label"`
	Kind     string                    `json:"kind"`
	Changes  []diagnosisChangeResource `json:"changes"`
	Hint     string                    `json:"hint,omitempty"`
	Redeploy bool                      `json:"redeploy,omitempty"`
}

type diagnosisCauseResource struct {
	Code        string                    `json:"code"`
	Title       string                    `json:"title"`
	Explanation string                    `json:"explanation"`
	Confidence  string                    `json:"confidence"`
	Evidence    []diagnosisSignalResource `json:"evidence"`
	Fixes       []diagnosisFixResource    `json:"fixes"`
}

func toCauseResources(causes []diagnose.Cause) []diagnosisCauseResource {
	out := make([]diagnosisCauseResource, 0, len(causes))
	for _, c := range causes {
		res := diagnosisCauseResource{
			Code: c.Code, Title: c.Title, Explanation: c.Explanation, Confidence: c.Confidence,
			Evidence: make([]diagnosisSignalResource, 0, len(c.Evidence)),
			Fixes:    make([]diagnosisFixResource, 0, len(c.Fixes)),
		}
		for _, e := range c.Evidence {
			res.Evidence = append(res.Evidence, diagnosisSignalResource{Source: e.Source, Excerpt: e.Excerpt})
		}
		for _, f := range c.Fixes {
			fr := diagnosisFixResource{N: f.N, Label: f.Label, Kind: f.Kind, Hint: f.Hint, Redeploy: f.Redeploy, Changes: make([]diagnosisChangeResource, 0, len(f.Changes))}
			for _, ch := range f.Changes {
				fr.Changes = append(fr.Changes, diagnosisChangeResource{Field: ch.Field, From: ch.From, To: ch.To, NeedsInput: ch.NeedsInput})
			}
			res.Fixes = append(res.Fixes, fr)
		}
		out = append(out, res)
	}
	return out
}

// anyPatchFix reports whether some cause carries a fix that can be applied
// without further input.
func anyPatchFix(causes []diagnose.Cause) bool {
	for _, c := range causes {
		for _, f := range c.Fixes {
			if f.Kind == diagnose.FixPatch {
				return true
			}
		}
	}
	return false
}
