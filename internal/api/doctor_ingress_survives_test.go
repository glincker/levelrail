package api

import (
	"strings"
	"testing"
	"time"

	ingressdriver "github.com/GLINCKER/levelrail/internal/ingress"
)

func TestDoctorIngressSurvives(t *testing.T) {
	now := time.Unix(1700000000, 0)
	snap := func(st ingressdriver.SnapshotStatus) func() ingressdriver.SnapshotStatus {
		return func() ingressdriver.SnapshotStatus { return st }
	}
	tests := []struct {
		name       string
		edge       *DoctorIngressEdge
		wantNone   bool
		wantStatus string
		wantIn     string
		wantFix    bool
	}{
		{name: "unset reports nothing", wantNone: true},
		{name: "no socket activation", edge: &DoctorIngressEdge{}, wantStatus: doctorStatusWarn, wantIn: "refuses every connection", wantFix: true},
		{name: "socket activation, no snapshot support", edge: &DoctorIngressEdge{SocketActivation: true}, wantStatus: doctorStatusWarn, wantIn: "no route snapshot"},
		{name: "snapshot off", edge: &DoctorIngressEdge{SocketActivation: true, Snapshot: snap(ingressdriver.SnapshotStatus{})}, wantStatus: doctorStatusWarn, wantIn: "snapshot is off"},
		{name: "save failing", edge: &DoctorIngressEdge{SocketActivation: true, Snapshot: snap(ingressdriver.SnapshotStatus{Enabled: true, SaveError: "disk full"})}, wantStatus: doctorStatusWarn, wantIn: "disk full"},
		{name: "restore failed and nothing saved since", edge: &DoctorIngressEdge{SocketActivation: true, Snapshot: snap(ingressdriver.SnapshotStatus{Enabled: true, RestoreError: "corrupt"})}, wantStatus: doctorStatusWarn, wantIn: "could not be used"},
		{name: "first boot, nothing saved yet", edge: &DoctorIngressEdge{SocketActivation: true, Snapshot: snap(ingressdriver.SnapshotStatus{Enabled: true})}, wantStatus: doctorStatusUnknown, wantIn: "not finished"},
		{name: "restore failed but a fresh snapshot exists", edge: &DoctorIngressEdge{SocketActivation: true, Snapshot: snap(ingressdriver.SnapshotStatus{Enabled: true, RestoreError: "corrupt", SavedAt: now.Add(-time.Minute)})}, wantStatus: doctorStatusOK, wantIn: "1m0s ago"},
		{name: "healthy", edge: &DoctorIngressEdge{SocketActivation: true, Snapshot: snap(ingressdriver.SnapshotStatus{Enabled: true, Restored: true, SavedAt: now.Add(-90 * time.Second)})}, wantStatus: doctorStatusOK, wantIn: "true on this install"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &Router{doctorEdge: tt.edge}
			got := rt.doctorCheckIngressSurvives(now)
			if tt.wantNone {
				if len(got) != 0 {
					t.Fatalf("got %+v", got)
				}
				return
			}
			if len(got) != 1 || got[0].Code != doctorIngressSurvivesCode || got[0].Status != tt.wantStatus || !strings.Contains(got[0].Message, tt.wantIn) {
				t.Fatalf("got %+v", got)
			}
			if (got[0].Fix != "") != tt.wantFix {
				t.Fatalf("fix = %q, want fix %v", got[0].Fix, tt.wantFix)
			}
		})
	}
}
