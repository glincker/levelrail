package walship

import (
	"fmt"
	"io"
	"strings"
)

// Segment is one shipped WAL segment found in a sink.
type Segment struct {
	Name       string
	Generation int
	Seq        uint32
	Offset     int64
}

// Reader lists and fetches what a Shipper wrote; promote and status build on it.
type Reader struct{ Sink Sink }

// Generations returns generation numbers that have a snapshot, ascending.
func (r Reader) Generations() ([]int, error) {
	names, err := r.Sink.List("gen-")
	if err != nil {
		return nil, fmt.Errorf("walship: list: %w", err)
	}
	var gens []int
	for _, n := range names {
		var g int
		if strings.HasSuffix(n, "/snapshot.db.gz") {
			if _, err := fmt.Sscanf(n, "gen-%d/", &g); err == nil {
				gens = append(gens, g)
			}
		}
	}
	return gens, nil
}

// Segments lists one generation's segments ordered by epoch then offset.
func (r Reader) Segments(gen int) ([]Segment, error) {
	prefix := fmt.Sprintf("gen-%06d/", gen)
	names, err := r.Sink.List(prefix)
	if err != nil {
		return nil, fmt.Errorf("walship: list: %w", err)
	}
	var out []Segment
	for _, n := range names {
		s := Segment{Name: n, Generation: gen}
		if _, err := fmt.Sscanf(strings.TrimPrefix(n, prefix), "e%d-%d.seg.gz", &s.Seq, &s.Offset); err == nil {
			out = append(out, s)
		}
	}
	return out, nil
}

// Fetch returns the decoded bytes of one object.
func (r Reader) Fetch(name string) ([]byte, error) {
	rc, err := r.Sink.Get(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	return io.ReadAll(rc)
}
