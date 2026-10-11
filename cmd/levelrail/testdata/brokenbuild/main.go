// Command brokenbuild stands in for a release that passes pre-flight but then
// writes a new schema version and never becomes healthy, so the self-upgrade
// live test can prove the automatic rollback.
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	_ "modernc.org/sqlite"
)

// The self-upgrade host command greps a downloaded binary for these usage
// strings before it will run it, the same check real releases satisfy.
const (
	versionUsage      = "usage: levelrail version [--json]"
	migrateCheckUsage = "usage: levelrail migrate-check --db <database copy> [--json]"
)

var (
	reportedVersion = "v0.6.0"
	baseSchema      = "0"
)

func main() {
	schema, _ := strconv.Atoi(baseSchema)
	args := os.Args[1:]
	switch {
	case len(args) >= 1 && args[0] == "version":
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"version": reportedVersion, "schema_version": schema + 1, "os": "test", "arch": "test"})
	case len(args) >= 1 && args[0] == "migrate-check":
		_ = json.NewEncoder(os.Stdout).Encode(map[string]int{"schema_before": schema, "schema_after": schema + 1})
	case len(args) >= 1 && args[0] == "help":
		fmt.Println(versionUsage, migrateCheckUsage)
	default:
		serveBroken(schema)
	}
}

func serveBroken(schema int) {
	dataDir := os.Getenv("APP_DATA_DIR")
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dataDir, "levelrail.db"))
	if err == nil {
		_, _ = db.Exec(`INSERT INTO schema_migrations (version, name) VALUES (?, 'broken_half_applied')`, schema+1)
		_, _ = db.Exec(`DELETE FROM live_probe`)
		_ = db.Close()
	}
	addr := os.Getenv("APP_HTTP_ADDR")
	fmt.Fprintln(os.Stderr, "brokenbuild: serving unhealthy on", addr)
	_ = http.ListenAndServe(addr, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unhealthy", http.StatusServiceUnavailable)
	}))
	os.Exit(1)
}
