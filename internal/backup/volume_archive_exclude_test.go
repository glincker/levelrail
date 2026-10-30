package backup

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type plainArchiver struct{ calls int }

func (p *plainArchiver) Archive(context.Context, string) (io.ReadCloser, error) {
	p.calls++
	return io.NopCloser(strings.NewReader("")), nil
}

type excludingArchiver struct {
	plainArchiver
	got []string
}

func (e *excludingArchiver) ArchiveExcluding(_ context.Context, _ string, ex []string) (io.ReadCloser, error) {
	e.got = ex
	return io.NopCloser(strings.NewReader("")), nil
}

func TestArchiveVolumeLeavesOutProtectedFiles(t *testing.T) {
	excl := func(_ context.Context, _, vol string) ([]string, error) {
		if vol == "mail_config" {
			return []string{"certs/managed*.key"}, nil
		}
		return nil, nil
	}
	plain := &plainArchiver{}
	r := &Runner{VolumeArchiver: plain, ArchiveExcludes: excl}
	if _, err := r.archiveVolume(context.Background(), "mail", "data", "v"); err != nil || plain.calls != 1 {
		t.Fatalf("unprotected volume: %v calls=%d", err, plain.calls)
	}
	if _, err := r.archiveVolume(context.Background(), "mail", "mail_config", "v"); !errors.Is(err, errCannotExclude) {
		t.Fatalf("an archiver that cannot exclude must fail closed: %v", err)
	}
	ex := &excludingArchiver{}
	r.VolumeArchiver = ex
	if _, err := r.archiveVolume(context.Background(), "mail", "mail_config", "v"); err != nil || len(ex.got) != 1 || ex.calls != 0 {
		t.Fatalf("excludes not passed: %v %v", err, ex.got)
	}
	r.ArchiveExcludes = func(context.Context, string, string) ([]string, error) { return nil, errors.New("db down") }
	if _, err := r.archiveVolume(context.Background(), "mail", "mail_config", "v"); err == nil {
		t.Fatal("an exclude lookup error must fail the backup")
	}
}
