package ingress

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
)

func TestPublishRequestHostOwners_SkipsPlatformRoutes(t *testing.T) {
	c := &Controller{}
	c.publishRequestHostOwners(map[string]string{
		"app.example.com":  "web",
		"dash.example.com": dashboardRouteOwner,
		"reg.example.com":  registryRouteOwner,
		"llm.example.com":  modelRouteOwner,
	})

	stats := ingress.DefaultRequestStats()
	stats.DrainByApp()
	for _, host := range []string{"app.example.com", "dash.example.com", "reg.example.com", "llm.example.com"} {
		stats.Observe(host, 200, time.Millisecond, 0, 0, false)
	}
	got := stats.DrainByApp()
	if len(got) != 1 || got["web"].Requests != 1 {
		t.Errorf("attributed = %v, want only the web app", got)
	}
}
