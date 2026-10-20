package api

import (
	"strings"
	"testing"
	"time"
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
			got := rt.doctorCheckIngressEdge()
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
