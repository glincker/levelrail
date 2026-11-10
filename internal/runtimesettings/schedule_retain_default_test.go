package runtimesettings

import "testing"

func TestScheduleRetainDefaultSeedsNoLimit(t *testing.T) {
	for _, d := range Definitions() {
		if d.Key == KeyBackupsScheduleRetainDefault {
			if d.Default != "0" || d.Env != "APP_BACKUP_SCHEDULE_RETAIN_DEFAULT" {
				t.Fatalf("unexpected seed: %+v", d)
			}
			return
		}
	}
	t.Fatal("setting not registered")
}
