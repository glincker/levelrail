package experimental

import (
	"strings"
	"testing"
)

func TestRuntimeToggle(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		feature Feature
		runtime bool
		want    bool
	}{
		{"off by default", "", ResourceAdvisor, false, false},
		{"settings turns it on", "", ResourceAdvisor, true, true},
		{"env wins over settings off", "resource-advisor", ResourceAdvisor, false, true},
		{"non toggleable ignores settings", "", IaC, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvVar, tc.env)
			Reset()
			defer Reset()
			SetRuntime(tc.feature, tc.runtime)
			if got := Enabled(tc.feature); got != tc.want {
				t.Fatalf("Enabled = %v, want %v", got, tc.want)
			}
			if EnabledByEnv(tc.feature) != (tc.env != "") {
				t.Fatalf("EnabledByEnv wrong for env %q", tc.env)
			}
		})
	}
}

func TestToggleableListAndMessage(t *testing.T) {
	if got := ToggleableList(); len(got) != 1 || got[0] != ResourceAdvisor {
		t.Fatalf("ToggleableList = %v", got)
	}
	if !strings.Contains(DisabledMessage(ResourceAdvisor), "Settings") {
		t.Fatal("beta message should point at Settings")
	}
	if strings.Contains(DisabledMessage(IaC), "Settings") {
		t.Fatal("env only feature should not point at Settings")
	}
}
