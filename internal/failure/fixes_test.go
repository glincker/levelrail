package failure

import "testing"

func TestClassifyAttachesBuildFixes(t *testing.T) {
	tests := []struct {
		name     string
		in       Input
		wantCode string
		wantKind string
	}{
		{
			name:     "transient network is a retry",
			in:       Input{Status: "failed", FailingStep: "build", LogLines: []string{"npm ERR! network ETIMEDOUT registry.npmjs.org"}},
			wantCode: "transient_network", wantKind: "rerun",
		},
		{
			name:     "peer dependency conflict is a source fix",
			in:       Input{Status: "failed", FailingStep: "build", LogLines: []string{"npm ERR! code ERESOLVE"}},
			wantCode: "node_peer_deps", wantKind: "edit_source",
		},
		{
			name:     "ansi colour does not hide a match",
			in:       Input{Status: "failed", FailingStep: "build", LogLines: []string{"\x1b[31mModuleNotFoundError: No module named 'flask'\x1b[0m"}},
			wantCode: "python_module_missing", wantKind: "edit_source",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, ok := Classify(tc.in, Options{})
			if !ok {
				t.Fatal("expected a failure")
			}
			for _, fx := range f.Fixes {
				if fx.Code == tc.wantCode && fx.Kind == tc.wantKind {
					return
				}
			}
			t.Errorf("fixes = %+v, want code %s kind %s", f.Fixes, tc.wantCode, tc.wantKind)
		})
	}
}

func TestClassifyNoBuildFixesOutsideBuild(t *testing.T) {
	f, ok := Classify(Input{Status: "failed", FailingStep: "health", LogLines: []string{"permission denied"}}, Options{})
	if !ok {
		t.Fatal("expected a failure")
	}
	if len(f.Fixes) != 0 {
		t.Errorf("fixes = %+v, want none outside the build phase", f.Fixes)
	}
}
