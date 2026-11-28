package appsleep

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/store"
)

type cappedSleep struct{ capped bool }

func (cappedSleep) SetAppSleeping(context.Context, string, bool, time.Time) error { return nil }

func (cappedSleep) GetAppSleep(context.Context, string) (store.AppSleep, error) {
	return store.AppSleep{Sleeping: true}, nil
}

func (c cappedSleep) SpendCapped(context.Context, string) (bool, error) { return c.capped, nil }

func TestWakeRefusedWhileSpendCapped(t *testing.T) {
	woke, err := Wake(context.Background(), cappedSleep{capped: true}, nil, nil, "web", time.Now())
	if !errors.Is(err, ErrCapped) || woke {
		t.Fatalf("woke=%v err=%v, want ErrCapped", woke, err)
	}
}
