package statuspage

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestMonthlyReport(t *testing.T) {
	comps := []store.StatusComponent{{ID: "c1", Kind: KindApp, Target: "blog", DisplayName: "Blog"}, {ID: "c2", Kind: KindApp, Target: "api", DisplayName: "API"}}
	daily := []store.StatusDaily{
		{ComponentID: "c1", Day: "2026-11-03", OK: 99, Down: 1},
		{ComponentID: "c1", Day: "2026-10-31", Down: 50},
	}
	rows, err := MonthlyReport(comps, daily, "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Component != "API" || rows[0].Uptime != nil {
		t.Fatalf("rows[0] = %+v", rows[0])
	}
	if rows[1].Uptime == nil || *rows[1].Uptime != 99 || !strings.Contains(rows[1].Summary, "99.00%") {
		t.Fatalf("rows[1] = %+v", rows[1])
	}
	if _, err := MonthlyReport(comps, daily, "nope"); err == nil {
		t.Fatal("want error for bad month")
	}
}
