package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

func TestImageSettings_KeepAndSchedule(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	put := func(key, val string) int {
		return rsDo(t, rt, cookie, http.MethodPut, rsBase+"/"+key, `{"value":`+val+`}`).Code
	}
	if got := rt.imageRetentionDefaults().KeepPerApp; got != 3 {
		t.Fatalf("default keep = %d, want 3", got)
	}
	cases := []struct {
		name, key, val string
		want           int
	}{
		{"keep below 1 refused", runtimesettings.KeyImagesKeepPerApp, `"0"`, http.StatusBadRequest},
		{"keep 5 accepted", runtimesettings.KeyImagesKeepPerApp, `"5"`, http.StatusOK},
		{"bad cron refused", runtimesettings.KeyImagesPruneSchedule, `"not a cron"`, http.StatusBadRequest},
		{"cron accepted", runtimesettings.KeyImagesPruneSchedule, `"0 3 * * 0"`, http.StatusOK},
		{"off accepted", runtimesettings.KeyImagesPruneSchedule, `"off"`, http.StatusOK},
	}
	for _, tc := range cases {
		if got := put(tc.key, tc.val); got != tc.want {
			t.Fatalf("%s: status %d, want %d", tc.name, got, tc.want)
		}
	}
	if got := rt.imageRetentionDefaults().KeepPerApp; got != 5 {
		t.Fatalf("keep after set = %d, want 5", got)
	}
}

func TestImagePruneDue(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	base := time.Date(2026, 11, 1, 2, 0, 0, 0, time.UTC)
	cases := []struct {
		name, sched string
		now         time.Time
		want        bool
	}{
		{"off never runs", "off", base.Add(48 * time.Hour), false},
		{"before the slot", "0 3 * * *", base.Add(30 * time.Minute), false},
		{"at the slot", "0 3 * * *", base.Add(time.Hour), true},
	}
	for _, tc := range cases {
		if rec := rsDo(t, rt, cookie, http.MethodPut, rsBase+"/"+runtimesettings.KeyImagesPruneSchedule, `{"value":"`+tc.sched+`"}`); rec.Code != http.StatusOK {
			t.Fatalf("%s: set = %d", tc.name, rec.Code)
		}
		if got := rt.imagePruneDue(t.Context(), base, tc.now); got != tc.want {
			t.Fatalf("%s: due = %v, want %v", tc.name, got, tc.want)
		}
	}
}
