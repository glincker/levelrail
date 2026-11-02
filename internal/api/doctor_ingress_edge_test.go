package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

func TestDoctorCheckIngressEdge(t *testing.T) {
	tests := []struct {
		name       string
		edge       *DoctorIngressEdge
		wantNone   bool
		wantStatus string
		wantIn     string
	}{
		{name: "unset reports nothing", wantNone: true},
		{name: "hardening off warns", edge: &DoctorIngressEdge{}, wantStatus: doctorStatusWarn, wantIn: "hardening is off"},
		{name: "no proxies, no sockets", edge: &DoctorIngressEdge{Hardening: true, RetryWindow: 3 * time.Second}, wantStatus: doctorStatusOK, wantIn: "no trusted proxies"},
		{name: "socket activation", edge: &DoctorIngressEdge{Hardening: true, SocketActivation: true, TrustedProxies: 2}, wantStatus: doctorStatusOK, wantIn: "restarts queue connections"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &Router{doctorEdge: tt.edge}
			got := rt.doctorCheckIngressEdge(t.Context())
			if tt.wantNone {
				if len(got) != 0 {
					t.Fatalf("got %+v", got)
				}
				return
			}
			if len(got) != 1 || got[0].Status != tt.wantStatus || !strings.Contains(got[0].Message, tt.wantIn) {
				t.Errorf("got %+v", got)
			}
		})
	}
}

func TestDoctorCheckIngressEdge_StoredBodyCap(t *testing.T) {
	_, db := newTestRouter(t)
	svc := runtimesettings.New(db, func(string) (string, bool) { return "", false })
	rt := &Router{doctorEdge: &DoctorIngressEdge{Hardening: true, MaxBodyBytes: 1 << 30}, runtimeSettings: svc}
	if _, err := svc.Set(t.Context(), runtimesettings.KeyIngressMaxBodyBytes, json.RawMessage(`1048576`), "test"); err != nil {
		t.Fatal(err)
	}
	got := rt.doctorCheckIngressEdge(t.Context())
	if len(got) != 1 || !strings.Contains(got[0].Message, "capped at 1048576 bytes") {
		t.Errorf("got %+v", got)
	}
}
