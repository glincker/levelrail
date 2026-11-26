package runtimesettings

import "testing"

func TestDBCrashLoopSettingsAreLive(t *testing.T) {
	want := map[string]string{KeyDBCrashLoopRestarts: "3", KeyDBCrashLoopWindowSeconds: "60"}
	found := 0
	for _, d := range Definitions() {
		def, ok := want[d.Key]
		if !ok {
			continue
		}
		found++
		if d.Default != def || d.Apply != ApplyLive || d.Env == "" {
			t.Errorf("%s: default %q apply %v env %q", d.Key, d.Default, d.Apply, d.Env)
		}
		if d.CheckValue("0") == nil {
			t.Errorf("%s accepts 0", d.Key)
		}
	}
	if found != len(want) {
		t.Fatalf("found %d of %d keys", found, len(want))
	}
}
