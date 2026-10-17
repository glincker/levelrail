package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/probe"
)

type fakeWakeWindow struct {
	a   store.AppSleep
	err error
}

func (f fakeWakeWindow) GetAppSleep(context.Context, string) (store.AppSleep, error) {
	return f.a, f.err
}

func TestWakeProbeConfig(t *testing.T) {
	const normal, fast = 5 * time.Second, 250 * time.Millisecond
	svc := &store.DesiredService{Name: "web"}
	tests := []struct {
		name   string
		window WakeWindowReader
		wake   time.Duration
		in     time.Duration
		want   time.Duration
	}{
		{"just woke", fakeWakeWindow{a: store.AppSleep{IdleMinutes: 30, Since: time.Now()}}, fast, normal, fast},
		{"already faster than the wake interval", fakeWakeWindow{a: store.AppSleep{IdleMinutes: 30, Since: time.Now()}}, fast, 100 * time.Millisecond, 100 * time.Millisecond},
		{"unset interval gets the wake interval", fakeWakeWindow{a: store.AppSleep{IdleMinutes: 30, Since: time.Now()}}, fast, 0, fast},
		{"woke long ago", fakeWakeWindow{a: store.AppSleep{IdleMinutes: 30, Since: time.Now().Add(-time.Hour)}}, fast, normal, normal},
		{"still asleep", fakeWakeWindow{a: store.AppSleep{IdleMinutes: 30, Sleeping: true, Since: time.Now()}}, fast, normal, normal},
		{"sleep off", fakeWakeWindow{a: store.AppSleep{Since: time.Now()}}, fast, normal, normal},
		{"lookup fails", fakeWakeWindow{err: errors.New("db")}, fast, normal, normal},
		{"no reader", nil, fast, normal, normal},
		{"interval disabled", fakeWakeWindow{a: store.AppSleep{IdleMinutes: 30, Since: time.Now()}}, 0, normal, normal},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &Controller{readyBudget: time.Minute}
			WithWakeProbe(tc.window, tc.wake)(c)
			if tc.window == nil {
				c.wakeWindow = nil
			}
			got := c.wakeProbeConfig(context.Background(), svc, probe.Config{Interval: tc.in})
			if got.Interval != tc.want {
				t.Errorf("interval = %s, want %s", got.Interval, tc.want)
			}
		})
	}
}
