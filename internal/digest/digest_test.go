package digest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

type fakeSource struct{ stats []Stats }

func (f fakeSource) ProjectStats(context.Context, time.Time, time.Time) ([]Stats, error) {
	return f.stats, nil
}

func (f fakeSource) ChannelsFor(_ context.Context, p string) ([]string, error) {
	return []string{p + "-a", p + "-bad"}, nil
}

type fakeSender struct{ got []string }

func (f *fakeSender) SendNotice(_ context.Context, id, text string) error {
	if strings.HasSuffix(id, "-bad") {
		return errors.New("down")
	}
	f.got = append(f.got, id+"|"+text)
	return nil
}

func TestRunOnceSkipsQuietAndSurvivesFailingChannel(t *testing.T) {
	snd := &fakeSender{}
	r := &Runner{
		Source: fakeSource{stats: []Stats{{Project: "quiet"}, {Project: "shop", Deploys: 3, Failures: 1, BackupsOK: 2}}},
		Sender: snd,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:    func() time.Time { return time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC) },
	}
	n, err := r.RunOnce(context.Background())
	if err != nil || n != 1 || len(snd.got) != 1 {
		t.Fatalf("n=%d err=%v got=%v", n, err, snd.got)
	}
	if !strings.Contains(snd.got[0], "Deploys: 3 (1 failed)") || !strings.Contains(snd.got[0], "2026-11-01") {
		t.Fatalf("unexpected text %q", snd.got[0])
	}
}

func TestRunOnceDisabled(t *testing.T) {
	snd := &fakeSender{}
	r := &Runner{Source: fakeSource{stats: []Stats{{Project: "a", Deploys: 1}}}, Sender: snd, Enabled: func() bool { return false }}
	if n, _ := r.RunOnce(context.Background()); n != 0 || len(snd.got) != 0 {
		t.Fatal("disabled runner sent")
	}
}
