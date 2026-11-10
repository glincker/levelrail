package store

import (
	"context"
	"fmt"
)

// DBSpaceTable is one table or index and the bytes its pages occupy.
type DBSpaceTable struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

// DBSpace is the page-level footprint of the control plane database.
type DBSpace struct {
	PageSize         int64          `json:"page_size"`
	PageCount        int64          `json:"page_count"`
	FreePages        int64          `json:"free_pages"`
	TotalBytes       int64          `json:"total_bytes"`
	ReclaimableBytes int64          `json:"reclaimable_bytes"`
	FreePercent      int            `json:"free_percent"`
	Tables           []DBSpaceTable `json:"tables"`
}

// SpaceReport reads page counts and the largest tables. The per-table list is
// left empty when the SQLite build lacks the dbstat table.
func (db *DB) SpaceReport(ctx context.Context, topN int) (DBSpace, error) {
	var out DBSpace
	for pragma, dst := range map[string]*int64{
		"page_size":      &out.PageSize,
		"page_count":     &out.PageCount,
		"freelist_count": &out.FreePages,
	} {
		if err := db.DB.QueryRowContext(ctx, "PRAGMA "+pragma).Scan(dst); err != nil {
			return DBSpace{}, fmt.Errorf("store: space report %s: %w", pragma, err)
		}
	}
	out.TotalBytes = out.PageSize * out.PageCount
	out.ReclaimableBytes = out.PageSize * out.FreePages
	if out.PageCount > 0 {
		out.FreePercent = int(out.FreePages * 100 / out.PageCount)
	}
	out.Tables = []DBSpaceTable{}
	rows, err := db.DB.QueryContext(ctx, "SELECT name, SUM(pgsize) FROM dbstat GROUP BY name ORDER BY 2 DESC LIMIT ?", topN)
	if err != nil {
		return out, nil
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var t DBSpaceTable
		if rows.Scan(&t.Name, &t.Bytes) == nil {
			out.Tables = append(out.Tables, t)
		}
	}
	return out, rows.Err()
}

// CompactDatabase rewrites the file to give free pages back to the disk. It
// holds the writer for the duration, so callers run it only on request.
func (db *DB) CompactDatabase(ctx context.Context) error {
	if _, err := db.DB.ExecContext(ctx, "VACUUM"); err != nil {
		return fmt.Errorf("store: compact database: %w", err)
	}
	return nil
}
