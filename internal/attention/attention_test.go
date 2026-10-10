package attention

import (
	"strings"
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

func TestBuild_UpdateAvailable(t *testing.T) {
	t.Parallel()
	latest := "v1.1.0"
	got := Build(Input{Updates: apiclient.UpdatesResource{
		CurrentVersion: "v1.0.0", LatestVersion: &latest, UpdateAvailable: true,
	}})
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1: %+v", len(got), got)
	}
	if got[0].Kind != "update" || got[0].Severity != Warning {
		t.Errorf("item = %+v, want kind=update severity=warning", got[0])
	}

	if got := Build(Input{Updates: apiclient.UpdatesResource{CurrentVersion: "v1.0.0"}}); len(got) != 0 {
		t.Errorf("no update available: len = %d, want 0: %+v", len(got), got)
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

func TestBuild_RankingAndWaitingItems(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	notAfter := now.Add(72 * time.Hour)
	tests := []struct {
		name string
		in   Input
		want []string
	}{
		{"expired login is dropped", Input{Now: now, Devices: []apiclient.DevicePendingLogin{{ClientName: "old", ExpiresAt: now.Add(-time.Minute)}}}, []string{}},
		{"waiting items lead their severity", Input{
			Now:       now,
			Devices:   []apiclient.DevicePendingLogin{{ClientName: "laptop", RequesterIP: "10.0.0.9", ExpiresAt: now.Add(5 * time.Minute)}},
			Approvals: []apiclient.DeployApprovalResource{{ServiceName: "web", Action: "deploy", Image: "web:2", RequestedBy: "u1"}},
			Certs:     []apiclient.CertificateResource{{Domain: "a.example", Status: "healthy", Renewal: "stalled", NotAfter: notAfter}},
			Doctor:    apiclient.SystemDoctorResource{Checks: []apiclient.DoctorCheckResource{{Name: "docker", Status: "fail"}, {Name: "disk", Status: "warn"}}},
		}, []string{"critical:doctor", "warning:device_login", "warning:approval", "warning:cert_renewal", "warning:doctor"}},
		{"never connected node", Input{Now: now, Nodes: []apiclient.NodeResource{{Name: "n1", Status: "pending", StatusReason: "enrolled_never_connected"}}},
			[]string{"warning:node_enroll"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := []string{}
			for _, it := range Build(tc.in) {
				got = append(got, it.Severity+":"+it.Kind)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("items = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDeviceLoginDetail_NeverCarriesACode(t *testing.T) {
	t.Parallel()
	now := time.Now()
	items := Build(Input{Now: now, Devices: []apiclient.DevicePendingLogin{{ClientName: "laptop", ExpiresAt: now.Add(3 * time.Minute)}}})
	if len(items) != 1 || !strings.Contains(items[0].Detail, "Settings > CLI access") || !strings.Contains(items[0].Detail, "expires in") {
		t.Fatalf("items = %+v", items)
	}
}
