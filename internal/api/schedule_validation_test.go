package api

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestValidateBackupScheduleRequest_RejectsNeverRunning(t *testing.T) {
	cases := []struct {
		sched string
		want  bool
	}{
		{"0 0 30 2 *", false},
		{"0 0 31 4 *", false},
		{"0 3 * * *", true},
		{"0 0 29 2 *", true},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		if got := validateBackupScheduleRequest(rec, "bkt_1", c.sched, 0, 0); got != c.want {
			t.Errorf("%s: ok=%v want %v", c.sched, got, c.want)
		}
		if !c.want && rec.Code != 400 {
			t.Errorf("%s: status %d, want 400", c.sched, rec.Code)
		}
	}
}

func TestImagePruneDue_LeapDayStillSchedules(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if rec := rsDo(t, rt, cookie, "PUT", rsBase+"/images.prune_schedule", `{"value":"0 0 29 2 *"}`); rec.Code != 200 {
		t.Fatalf("set = %d", rec.Code)
	}
	last := time.Date(2028, 2, 29, 0, 0, 0, 0, time.UTC)
	if rt.imagePruneDue(t.Context(), last, last.Add(time.Hour)) {
		t.Fatal("due again on the same leap day")
	}
	if !rt.imagePruneDue(t.Context(), last, time.Date(2032, 2, 29, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("next leap day not due")
	}
}
