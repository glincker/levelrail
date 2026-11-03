package deploy

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestValidateFreezeWindow_RejectsNeverMatching(t *testing.T) {
	for _, tz := range []string{"", "America/New_York"} {
		w := store.DeployFreezeWindow{Cron: "0 0 30 2 *", Duration: time.Hour, Timezone: tz}
		if ValidateFreezeWindow(w) == nil {
			t.Errorf("tz %q: impossible freeze accepted", tz)
		}
	}
	leap := store.DeployFreezeWindow{Cron: "0 0 29 2 *", Duration: time.Hour}
	if err := ValidateFreezeWindow(leap); err != nil {
		t.Fatal(err)
	}
}
