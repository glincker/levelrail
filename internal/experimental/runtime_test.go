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
	if got := ToggleableList(); len(got) != 7 || got[0] != AIChat || got[1] != Canvas || got[2] != CLIConsole || got[3] != GitPush || got[4] != MobileAPI || got[5] != ResourceAdvisor || got[6] != SAMLSCIM {
		t.Fatalf("ToggleableList = %v", got)
	}
	for _, f := range []Feature{AIChat, Canvas, CLIConsole, GitPush, MobileAPI, ResourceAdvisor, SAMLSCIM} {
		if !strings.Contains(DisabledMessage(f), "Settings") {
			t.Fatalf("%s beta message should point at Settings", f)
		}
	}
	if !strings.Contains(DisabledMessage(IaC), "Settings") {
		t.Fatal("every feature message should point at Settings")
	}
}

func TestPendingForStartupWiredFeature(t *testing.T) {
	Set()
	defer Reset()
	if Pending(IaC) {
		t.Fatal("nothing saved yet")
	}
	SetRuntime(IaC, true)
	if Enabled(IaC) || !Pending(IaC) {
		t.Fatalf("saved on must wait for restart: enabled=%v pending=%v", Enabled(IaC), Pending(IaC))
	}
	ApplyAtBoot(IaC, true)
	if !Enabled(IaC) || Pending(IaC) {
		t.Fatal("boot applies the saved choice")
	}
	SetRuntime(IaC, false)
	if !Enabled(IaC) || !Pending(IaC) {
		t.Fatal("switching off also waits for restart")
	}
	SetRuntime(AIChat, true)
	if !Enabled(AIChat) || Pending(AIChat) {
		t.Fatal("live features apply at once")
	}
}
