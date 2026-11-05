package api

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeDoctorChannels struct {
	NotificationChannels
	rows []alerting.NotificationChannel
	err  error
}

func (f fakeDoctorChannels) ListNotificationChannels(context.Context) ([]alerting.NotificationChannel, error) {
	return f.rows, f.err
}

type fakeDoctorProviders struct {
	EmailProviderStore
	rows []store.EmailProvider
	err  error
}

func (f fakeDoctorProviders) ListEmailProviders(context.Context) ([]store.EmailProvider, error) {
	return f.rows, f.err
}

func TestDoctorChannels(t *testing.T) {
	one := []alerting.NotificationChannel{{ID: "c1"}}
	tests := []struct {
		name string
		rt   *Router
		want string
	}{
		{"nothing configured warns", &Router{notificationChannels: fakeDoctorChannels{}, emailProviders: fakeDoctorProviders{}}, doctorStatusWarn},
		{"nil surfaces warn", &Router{}, doctorStatusWarn},
		{"channel ok", &Router{notificationChannels: fakeDoctorChannels{rows: one}, emailProviders: fakeDoctorProviders{}}, doctorStatusOK},
		{"enabled provider ok", &Router{notificationChannels: fakeDoctorChannels{}, emailProviders: fakeDoctorProviders{rows: []store.EmailProvider{{Enabled: true}}}}, doctorStatusOK},
		{"disabled provider warns", &Router{notificationChannels: fakeDoctorChannels{}, emailProviders: fakeDoctorProviders{rows: []store.EmailProvider{{Enabled: false}}}}, doctorStatusWarn},
		{"channel error unknown", &Router{notificationChannels: fakeDoctorChannels{err: errors.New("boom")}}, doctorStatusUnknown},
		{"provider error unknown", &Router{notificationChannels: fakeDoctorChannels{}, emailProviders: fakeDoctorProviders{err: errors.New("boom")}}, doctorStatusUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.rt.doctorCheckNotifyReachable(context.Background())
			if got.Status != tt.want {
				t.Fatalf("status = %q, want %q (%s)", got.Status, tt.want, got.Message)
			}
			if tt.want == doctorStatusWarn && got.Fix == "" {
				t.Fatal("warn must carry a fix")
			}
		})
	}
}
