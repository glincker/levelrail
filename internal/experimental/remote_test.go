package experimental

import "testing"

func TestApplyRemoteFollowsServerButKeepsEnvOverride(t *testing.T) {
	t.Setenv(EnvVar, string(Plugins))
	Reset()
	t.Cleanup(Reset)

	ApplyRemote(map[Feature]bool{IaC: true, Plugins: false})
	if !Enabled(IaC) {
		t.Fatal("server enabled IaC should be on")
	}
	if !Enabled(Plugins) {
		t.Fatal("env override must stay on when the server says off")
	}
	if Enabled(AIModels) {
		t.Fatal("unlisted feature should be off")
	}
	ApplyRemote(map[Feature]bool{})
	if Enabled(IaC) {
		t.Fatal("server switch off should turn IaC off")
	}
}
