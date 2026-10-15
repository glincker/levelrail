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
	if got := ToggleableList(); len(got) != 5 || got[0] != AIChat || got[1] != CLIConsole || got[2] != MobileAPI || got[3] != ResourceAdvisor || got[4] != SAMLSCIM {
		t.Fatalf("ToggleableList = %v", got)
	}
	for _, f := range []Feature{AIChat, CLIConsole, MobileAPI, ResourceAdvisor, SAMLSCIM} {
		if !strings.Contains(DisabledMessage(f), "Settings") {
			t.Fatalf("%s beta message should point at Settings", f)
		}
	}
	if strings.Contains(DisabledMessage(IaC), "Settings") {
		t.Fatal("env only feature should not point at Settings")
	}
}
