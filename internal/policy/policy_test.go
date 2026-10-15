package policy

import "testing"

func TestEvaluate(t *testing.T) {
	healthy := Subject{Image: "ghcr.io/acme/web:1", MemoryBytes: 256 << 20, NanoCPUs: 500_000_000, HasReadiness: true, Replicas: 2}
	tests := []struct {
		name string
		set  Set
		subj Subject
		want []string
	}{
		{"empty set passes", Set{}, healthy, nil},
		{"off ignored", Set{RuleAllowedRegistries: {Mode: ModeOff, Values: []string{"x.io"}}}, healthy, nil},
		{"registry denied", Set{RuleAllowedRegistries: {Mode: ModeEnforce, Values: []string{"registry.acme.io"}}}, healthy, []string{CodeRegistryNotAllowed}},
		{"registry allowed", Set{RuleAllowedRegistries: {Mode: ModeEnforce, Values: []string{"GHCR.io"}}}, healthy, nil},
		{"bare image is docker.io", Set{RuleAllowedRegistries: {Mode: ModeEnforce, Values: []string{"docker.io"}}}, Subject{Image: "nginx:1"}, nil},
		{"readiness missing", Set{RuleRequireReadiness: {Mode: ModeWarn}}, Subject{}, []string{CodeReadinessRequired}},
		{"memory over", Set{RuleMaxMemoryMB: {Mode: ModeEnforce, Limit: 128}}, healthy, []string{CodeMemoryCeiling}},
		{"memory unset", Set{RuleMaxMemoryMB: {Mode: ModeEnforce, Limit: 512}}, Subject{}, []string{CodeMemoryCeiling}},
		{"cpu over", Set{RuleMaxCPU: {Mode: ModeEnforce, Limit: 0.25}}, healthy, []string{CodeCPUCeiling}},
		{"cpu ok", Set{RuleMaxCPU: {Mode: ModeEnforce, Limit: 1}}, healthy, nil},
		{"privileged", Set{RuleForbidPrivileged: {Mode: ModeEnforce}}, Subject{Privileged: true}, []string{CodePrivilegedForbidden}},
		{"host mount denied", Set{RuleForbidHostMounts: {Mode: ModeEnforce}}, Subject{HostMounts: []string{"/var/run/docker.sock"}}, []string{CodeHostMountForbidden}},
		{"host mount allowed prefix", Set{RuleForbidHostMounts: {Mode: ModeEnforce, Values: []string{"/srv/data"}}}, Subject{HostMounts: []string{"/srv/data/app"}}, nil},
		{"prefix is not substring", Set{RuleForbidHostMounts: {Mode: ModeEnforce, Values: []string{"/srv/data"}}}, Subject{HostMounts: []string{"/srv/database"}}, []string{CodeHostMountForbidden}},
		{"labels missing", Set{RuleRequiredLabels: {Mode: ModeEnforce, Values: []string{"team", "cost-center"}}}, Subject{Labels: map[string]string{"team": "a"}}, []string{CodeLabelRequired}},
		{"replicas over", Set{RuleMaxReplicas: {Mode: ModeEnforce, Limit: 1}}, healthy, []string{CodeReplicasExceeded}},
		{"default replica counts as one", Set{RuleMaxReplicas: {Mode: ModeEnforce, Limit: 1}}, Subject{}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, v := range Evaluate(tc.set, tc.subj) {
				got = append(got, v.Code)
				if v.NextAction == "" || v.Message == "" {
					t.Errorf("violation %s lacks message or next action", v.Code)
				}
			}
			if len(got) != len(tc.want) {
				t.Fatalf("codes = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("codes = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestMergeMostSpecificWins(t *testing.T) {
	org := Set{RuleMaxReplicas: {Mode: ModeEnforce, Limit: 2}, RuleRequireReadiness: {Mode: ModeWarn}}
	env := Set{RuleMaxReplicas: {Mode: ModeOff}}
	got := Merge(org, Set{}, env)
	if got[RuleMaxReplicas].Mode != ModeOff || got[RuleRequireReadiness].Mode != ModeWarn {
		t.Fatalf("merge = %+v", got)
	}
}

func TestBlocking(t *testing.T) {
	vs := Evaluate(Set{RuleRequireReadiness: {Mode: ModeWarn}, RuleForbidPrivileged: {Mode: ModeEnforce}}, Subject{Privileged: true})
	if len(vs) != 2 || len(Blocking(vs)) != 1 || Blocking(vs)[0].Code != CodePrivilegedForbidden {
		t.Fatalf("violations = %+v", vs)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name string
		set  Set
		ok   bool
	}{
		{"valid", Set{RuleMaxMemoryMB: {Mode: ModeEnforce, Limit: 512}}, true},
		{"unknown rule", Set{"nope": {Mode: ModeWarn}}, false},
		{"bad mode", Set{RuleForbidPrivileged: {Mode: "block"}}, false},
		{"ceiling needs limit", Set{RuleMaxCPU: {Mode: ModeEnforce}}, false},
		{"registries need values", Set{RuleAllowedRegistries: {Mode: ModeWarn}}, false},
		{"off needs nothing", Set{RuleMaxCPU: {Mode: ModeOff}}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := Validate(tc.set); (err == nil) != tc.ok {
				t.Fatalf("err = %v, ok want %v", err, tc.ok)
			}
		})
	}
}
