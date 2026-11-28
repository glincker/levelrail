package updatecheck

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

type countingAnnouncer struct{ polls int }

func (c *countingAnnouncer) Poll(context.Context) error { c.polls++; return nil }

type disabledStore struct{}

func (disabledStore) GetUpdateSettings(context.Context) (store.UpdateSettings, error) {
	return store.UpdateSettings{AutoUpdateEnabled: false}, nil
}

func TestTickPollsAnnouncementsEvenWhenAutoUpdateIsOff(t *testing.T) {
	a := &countingAnnouncer{}
	s := &Scheduler{Store: disabledStore{}, Announcements: a}
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if a.polls != 1 {
		t.Fatalf("announcements polled %d times, want 1", a.polls)
	}
}
