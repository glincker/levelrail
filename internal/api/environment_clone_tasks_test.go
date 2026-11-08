package api

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestCopyScheduledTasksKeepsLimits(t *testing.T) {
	rt, db := newTestRouter(t)
	seedScheduledTaskApp(t, db, "web")
	seedScheduledTaskApp(t, db, "web-copy")
	now := time.Now().UTC()
	src := store.ScheduledTask{
		ID: "sct_src", ServiceName: "web", Command: []string{"bin/report"}, Schedule: "0 3 * * *", Enabled: true,
		Name: "nightly", Mode: store.ScheduledTaskModeFresh, Timezone: "Europe/Berlin", TimeoutSeconds: 90,
		CPULimitMillis: 250, MemoryLimitMB: 128, MaxRetries: 2, RetryBackoffSeconds: 30,
		Source: store.ScheduledTaskSourceAPI, LastRunOutput: "old", CreatedAt: now, UpdatedAt: now,
	}
	if err := db.SaveScheduledTask(t.Context(), src); err != nil {
		t.Fatal(err)
	}
	if err := rt.copyScheduledTasks(t.Context(), "web", "web-copy"); err != nil {
		t.Fatal(err)
	}
	got, err := db.ListScheduledTasksForService(t.Context(), "web-copy")
	if err != nil || len(got) != 1 {
		t.Fatalf("copied %d task(s), err %v", len(got), err)
	}
	c := got[0]
	if c.ID == src.ID || c.Name != src.Name || c.Mode != src.Mode || c.Timezone != src.Timezone ||
		c.TimeoutSeconds != 90 || c.CPULimitMillis != 250 || c.MemoryLimitMB != 128 ||
		c.MaxRetries != 2 || c.RetryBackoffSeconds != 30 || c.Source != src.Source || c.LastRunOutput != "" {
		t.Fatalf("clone lost fields: %+v", c)
	}
}
