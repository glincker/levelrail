package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestDiffEnv(t *testing.T) {
	res := diffEnv("stg", map[string]string{"A": "1", "B": "2", "ONLY_A": "x", "P": "plain"}, []string{"DB", "S_ONLY_A", "P2"},
		"prod", map[string]string{"A": "1", "B": "3", "ONLY_B": "y", "P2": "now-plain"}, []string{"DB", "P"})

	if res.Same != 2 {
		t.Errorf("same = %d, want 2 (A and the shared secret DB)", res.Same)
	}
	if len(res.OnlyA) != 2 || res.OnlyA[0].Key != "ONLY_A" || res.OnlyA[1].Key != "S_ONLY_A" || !res.OnlyA[1].Secret || res.OnlyA[1].A != "" {
		t.Errorf("only_a = %+v", res.OnlyA)
	}
	if len(res.OnlyB) != 1 || res.OnlyB[0].Key != "ONLY_B" || res.OnlyB[0].B != "y" {
		t.Errorf("only_b = %+v", res.OnlyB)
	}
	got := map[string]envDiffEntry{}
	for _, d := range res.Differ {
		got[d.Key] = d
	}
	if got["B"].A != "2" || got["B"].B != "3" {
		t.Errorf("B diff = %+v", got["B"])
	}
	if !got["P"].Secret || got["P"].A != "" || got["P"].B != "" || !got["P2"].Secret {
		t.Errorf("mixed secret/plain keys must differ without leaking a value: P=%+v P2=%+v", got["P"], got["P2"])
	}
}

func TestPrintEnvDiff_NeverPrintsSecretValues(t *testing.T) {
	res := diffEnv("a", map[string]string{"TOKEN": "hunter2"}, []string{"TOKEN"}, "b", map[string]string{}, nil)
	var out bytes.Buffer
	printEnvDiff(&out, res)
	if strings.Contains(out.String(), "hunter2") || !strings.Contains(out.String(), "- TOKEN (secret)") {
		t.Errorf("output = %q", out.String())
	}
}
