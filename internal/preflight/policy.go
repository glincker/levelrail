package preflight

import "github.com/GLINCKER/levelrail/internal/policy"

// checkPolicy turns guardrail violations into checks: enforce fails, warn
// warns. A clean subject adds no check, so apps without rules see no noise.
func checkPolicy(req Request, env Env) []Check {
	if len(env.Policy) == 0 {
		return nil
	}
	subj := policy.Subject{
		Image: req.Image, MemoryBytes: req.MemoryBytes, NanoCPUs: req.NanoCPUs,
		HasReadiness: req.HasReadiness, Privileged: req.Privileged, HostMounts: req.BindMounts,
		Labels: req.Labels, Replicas: req.Replicas,
	}
	var out []Check
	for _, v := range policy.Evaluate(env.Policy, subj) {
		st := StatusWarn
		if v.Blocks() {
			st = StatusFail
		}
		out = append(out, Check{ID: "policy." + v.Rule, Name: "Deploy policy", Status: st, Reason: v.Message, Fix: v.NextAction, Code: v.Code})
	}
	return out
}
