package mailbackup

import (
	"context"
	"encoding/json"
	"io"
	"sort"
	"strings"
)

// List reads every manifest in the sink, newest first. Unreadable manifests
// are skipped so one damaged file does not hide the rest.
func List(ctx context.Context, sink Sink) ([]Manifest, error) {
	names, err := sink.List(ctx)
	if err != nil {
		return nil, err
	}
	var out []Manifest
	for _, n := range names {
		if !strings.HasSuffix(n, ".manifest.json") {
			continue
		}
		rc, err := sink.Open(ctx, n)
		if err != nil {
			continue
		}
		var m Manifest
		err = json.NewDecoder(io.LimitReader(rc, 1<<16)).Decode(&m)
		_ = rc.Close()
		if err == nil && m.ID != "" {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// UsedBytes sums the size of every object in a local sink.
func (s *LocalSink) UsedBytes() (int64, error) {
	es, err := s.entries()
	if err != nil {
		return 0, err
	}
	var n int64
	for _, e := range es {
		n += e.size
	}
	return n, nil
}
