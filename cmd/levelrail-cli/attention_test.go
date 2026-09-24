package main

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func appEntry(name, variant string) apiclient.AppStatusEntry {
	var e apiclient.AppStatusEntry
	e.Name = name
	e.Status.Label = "Attention needed"
	e.Status.Variant = variant
	return e
}

func TestBuildAttentionItems(t *testing.T) {
	t.Parallel()
	notAfter := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		apps   []apiclient.AppStatusEntry
		nodes  []nodeResource
		certs  []apiclient.CertificateResource
		doc    systemDoctorResource
		failed []apiclient.FailedDeployResource
		status apiclient.SystemStatusResource
		want   []string
	}{
		{name: "healthy", apps: []apiclient.AppStatusEntry{appEntry("a", "success")}, want: []string{}},
		{
			name:  "critical sorts before warning",
			apps:  []apiclient.AppStatusEntry{appEntry("web", "destructive")},
			nodes: []nodeResource{{Name: "n1", Status: "offline"}, {Name: "n2", Status: "cordoned"}},
			certs: []apiclient.CertificateResource{
				{Domain: "a.io", Status: "expiring_soon", NotAfter: notAfter},
				{Domain: "b.io", Status: "expired", NotAfter: notAfter},
			},
			doc: systemDoctorResource{Checks: []doctorCheckResource{
				{Name: "Disk", Status: "warn"},
				{Name: "Docker", Status: "fail"},
				{Name: "Net", Status: "unknown"},
			}},
			want: []string{"critical:web", "critical:n1", "critical:b.io", "critical:Docker", "warning:a.io", "warning:Disk"},
		},
		{
			name:   "failed deploy and critical disk sort first",
			failed: []apiclient.FailedDeployResource{{DeployAttemptResource: apiclient.DeployAttemptResource{ServiceName: "api", Error: "build failed"}}},
			status: apiclient.SystemStatusResource{DataDirTotalBytes: 100, DataDirFreeBytes: 4},
			want:   []string{"critical:data dir", "critical:api"},
		},
		{
			name:   "warning disk at 8 percent free",
			status: apiclient.SystemStatusResource{DataDirTotalBytes: 100, DataDirFreeBytes: 8},
			want:   []string{"warning:data dir"},
		},
		{
			name:   "disk at 10 percent free is fine",
			status: apiclient.SystemStatusResource{DataDirTotalBytes: 100, DataDirFreeBytes: 10},
			want:   []string{},
		},
		{
			name:   "unknown disk size is ignored",
			status: apiclient.SystemStatusResource{},
			want:   []string{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildAttentionItems(attentionInput{Apps: tc.apps, Nodes: tc.nodes, Certs: tc.certs, Doctor: tc.doc, Failed: tc.failed, Status: tc.status})
			if len(got) != len(tc.want) {
				t.Fatalf("got %d items, want %d: %+v", len(got), len(tc.want), got)
			}
			for i, it := range got {
				if key := it.Severity + ":" + it.Subject; key != tc.want[i] {
					t.Errorf("item %d = %s, want %s", i, key, tc.want[i])
				}
			}
		})
	}
}
