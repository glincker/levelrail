package attention

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestBuild(t *testing.T) {
	t.Parallel()
	var bad apiclient.AppStatusEntry
	bad.Name = "web"
	bad.Status.Variant = "destructive"
	seen := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	notAfter := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	got := Build(Input{
		Apps:   []apiclient.AppStatusEntry{bad},
		Nodes:  []apiclient.NodeResource{{Name: "n1", Status: "offline", LastSeenAt: &seen}, {Name: "n2", Status: "offline"}},
		Certs:  []apiclient.CertificateResource{{Domain: "a.example", Status: "expiring_soon", NotAfter: notAfter}, {Domain: "b.example", Status: "expired", NotAfter: notAfter}},
		Doctor: apiclient.SystemDoctorResource{Checks: []apiclient.DoctorCheckResource{{Name: "disk", Status: "warn"}, {Name: "docker", Status: "fail"}}},
	})
	if len(got) != 7 {
		t.Fatalf("len = %d, want 7: %+v", len(got), got)
	}
	seenWarning := false
	for _, it := range got {
		if it.Severity == Warning {
			seenWarning = true
		} else if seenWarning {
			t.Errorf("critical item %+v after a warning", it)
		}
	}
	if got := Build(Input{}); len(got) != 0 || got == nil {
		t.Errorf("healthy result = %#v, want empty non-nil slice", got)
	}
}

func TestBuild_DiskAndFailedDeploys(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		free     int64
		wantSev  string
		wantNone bool
	}{
		{"plenty", 50, "", true},
		{"warn under 10 percent", 8, Warning, false},
		{"critical under 5 percent", 4, Critical, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Build(Input{Status: apiclient.SystemStatusResource{DataDirTotalBytes: 100, DataDirFreeBytes: tc.free}})
			if tc.wantNone {
				if len(got) != 0 {
					t.Fatalf("got %+v, want none", got)
				}
				return
			}
			if len(got) != 1 || got[0].Kind != "disk" || got[0].Severity != tc.wantSev {
				t.Errorf("got %+v, want one %s disk item", got, tc.wantSev)
			}
		})
	}

	failed := []apiclient.FailedDeployResource{{DeployAttemptResource: apiclient.DeployAttemptResource{ServiceName: "api", Error: "build failed"}}}
	got := Build(Input{Failed: failed})
	if len(got) != 1 || got[0].Kind != "deploy" || got[0].Subject != "api" || got[0].Detail != "build failed" {
		t.Errorf("failed deploy item = %+v", got)
	}
}
