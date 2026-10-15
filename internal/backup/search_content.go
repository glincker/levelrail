package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// SearchContent maps each index or collection to its document or point
// count, as the engine's own API reports it.
type SearchContent map[string]int64

// Encode is the canonical JSON form stored with a backup.
func (c SearchContent) Encode() string {
	b, err := json.Marshal(map[string]int64(c))
	if err != nil {
		return ""
	}
	return string(b)
}

// DecodeSearchContent parses Encode's output.
func DecodeSearchContent(s string) (SearchContent, error) {
	var c SearchContent
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		return nil, fmt.Errorf("decode content summary: %w", err)
	}
	return c, nil
}

// Total is the sum of every object's count.
func (c SearchContent) Total() int64 {
	var n int64
	for _, v := range c {
		n += v
	}
	return n
}

// Diff describes how got differs from c, or "" when they match exactly.
func (c SearchContent) Diff(got SearchContent) string {
	names := map[string]bool{}
	for k := range c {
		names[k] = true
	}
	for k := range got {
		names[k] = true
	}
	sorted := make([]string, 0, len(names))
	for k := range names {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	var diffs []string
	for _, k := range sorted {
		want, wok := c[k]
		have, hok := got[k]
		switch {
		case !hok:
			diffs = append(diffs, fmt.Sprintf("%q missing after restore (backup had %d)", k, want))
		case !wok:
			diffs = append(diffs, fmt.Sprintf("%q not in the backup (restored %d)", k, have))
		case want != have:
			diffs = append(diffs, fmt.Sprintf("%q has %d, backup had %d", k, have, want))
		}
	}
	return strings.Join(diffs, "; ")
}

// ReadSearchContent asks the engine running in containerName for its
// per-index (Meilisearch), per-collection exact point (Qdrant) or
// per-collection document (Typesense) counts.
func ReadSearchContent(ctx context.Context, rt docker.Runtime, engine, containerName string) (SearchContent, error) {
	out := SearchContent{}
	switch engine {
	case store.EngineMeilisearch:
		var stats struct {
			Indexes map[string]struct {
				NumberOfDocuments int64 `json:"numberOfDocuments"`
			} `json:"indexes"`
		}
		if err := searchAPI(ctx, rt, engine, containerName, "GET", "/stats", "", &stats); err != nil {
			return nil, err
		}
		for name, ix := range stats.Indexes {
			out[name] = ix.NumberOfDocuments
		}
	case store.EngineQdrant:
		var list struct {
			Result struct {
				Collections []struct {
					Name string `json:"name"`
				} `json:"collections"`
			} `json:"result"`
		}
		if err := searchAPI(ctx, rt, engine, containerName, "GET", "/collections", "", &list); err != nil {
			return nil, err
		}
		for _, c := range list.Result.Collections {
			var count struct {
				Result struct {
					Count int64 `json:"count"`
				} `json:"result"`
			}
			path := "/collections/" + url.PathEscape(c.Name) + "/points/count"
			if err := searchAPI(ctx, rt, engine, containerName, "POST", path, `{"exact":true}`, &count); err != nil {
				return nil, err
			}
			out[c.Name] = count.Result.Count
		}
	case store.EngineTypesense:
		var cols []struct {
			Name         string `json:"name"`
			NumDocuments int64  `json:"num_documents"`
		}
		if err := searchAPI(ctx, rt, engine, containerName, "GET", "/collections", "", &cols); err != nil {
			return nil, err
		}
		for _, c := range cols {
			out[c.Name] = c.NumDocuments
		}
	default:
		return nil, fmt.Errorf("engine %q has no content counts", engine)
	}
	return out, nil
}
