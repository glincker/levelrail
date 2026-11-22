package apiclient

import (
	"context"
	"net/http"
)

// DBSpaceTable is one table row of the database space report.
type DBSpaceTable struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

// DBSpacePart is the footprint of one prunable group of tables.
type DBSpacePart struct {
	ID             string   `json:"id"`
	Bytes          int64    `json:"bytes"`
	CleanupTargets []string `json:"cleanup_targets,omitempty"`
	Fix            string   `json:"fix,omitempty"`
}

// DBSpace is GET /api/v1/system/db-space.
type DBSpace struct {
	PageSize         int64          `json:"page_size"`
	PageCount        int64          `json:"page_count"`
	FreePages        int64          `json:"free_pages"`
	TotalBytes       int64          `json:"total_bytes"`
	ReclaimableBytes int64          `json:"reclaimable_bytes"`
	FreePercent      int            `json:"free_percent"`
	Tables           []DBSpaceTable `json:"tables"`
	Parts            []DBSpacePart  `json:"parts"`
}

// DBCompact is POST /api/v1/system/db-space/compact.
type DBCompact struct {
	Compacted   bool    `json:"compacted"`
	Reason      string  `json:"reason"`
	BytesBefore int64   `json:"bytes_before"`
	BytesAfter  int64   `json:"bytes_after"`
	Space       DBSpace `json:"space"`
}

// HostProcess is one row of GET /api/v1/system/processes.
type HostProcess struct {
	PID      int    `json:"pid"`
	Name     string `json:"name"`
	RSSBytes int64  `json:"rss_bytes"`
	Threads  int    `json:"threads"`
}

// SystemProcesses is GET /api/v1/system/processes.
type SystemProcesses struct {
	Supported bool          `json:"supported"`
	Total     int           `json:"total"`
	Processes []HostProcess `json:"processes"`
	Docker    *struct {
		Images       int64 `json:"images_bytes"`
		Containers   int64 `json:"containers_bytes"`
		Volumes      int64 `json:"volumes_bytes"`
		BuildCache   int64 `json:"build_cache_bytes"`
		ReclaimBytes int64 `json:"reclaimable_bytes"`
	} `json:"docker_disk"`
}

// GetDBSpace calls GET /api/v1/system/db-space.
func (c *Client) GetDBSpace(ctx context.Context) (DBSpace, error) {
	return getJSON[DBSpace](ctx, c, "/api/v1/system/db-space")
}

// CompactDB calls POST /api/v1/system/db-space/compact.
func (c *Client) CompactDB(ctx context.Context, force bool) (DBCompact, error) {
	var out DBCompact
	err := c.do(ctx, http.MethodPost, "/api/v1/system/db-space/compact", map[string]bool{"force": force}, &out)
	return out, err
}

// GetSystemProcesses calls GET /api/v1/system/processes.
func (c *Client) GetSystemProcesses(ctx context.Context) (SystemProcesses, error) {
	return getJSON[SystemProcesses](ctx, c, "/api/v1/system/processes")
}
