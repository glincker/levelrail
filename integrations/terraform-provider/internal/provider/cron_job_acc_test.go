package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// fakeJobAPI is an in-process stand-in for the /jobs routes with the server defaults applied.
type fakeJobAPI struct {
	mu   sync.Mutex
	jobs map[string]wireCronJob
	next int
}

func (f *fakeJobAPI) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rest, ok := strings.CutPrefix(r.URL.Path, "/api/v1/apps/web/jobs")
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	id := strings.TrimPrefix(rest, "/")
	switch {
	case r.Method == http.MethodGet && id == "":
		out := make([]wireCronJob, 0, len(f.jobs))
		for _, j := range f.jobs {
			out = append(out, j)
		}
		_ = json.NewEncoder(w).Encode(out)
	case r.Method == http.MethodPost && id == "":
		var in wireCronJob
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.next++
		in.ID = fmt.Sprintf("j%d", f.next)
		f.jobs[in.ID] = withJobDefaults(in)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(f.jobs[in.ID])
	case r.Method == http.MethodGet || r.Method == http.MethodPut || r.Method == http.MethodDelete:
		cur, found := f.jobs[id]
		if !found {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "scheduled task not found"})
			return
		}
		if r.Method == http.MethodDelete {
			delete(f.jobs, id)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == http.MethodPut {
			var in wireCronJob
			_ = json.NewDecoder(r.Body).Decode(&in)
			in.ID = cur.ID
			cur = withJobDefaults(in)
			f.jobs[id] = cur
		}
		_ = json.NewEncoder(w).Encode(cur)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func withJobDefaults(j wireCronJob) wireCronJob {
	if j.Mode == "" {
		j.Mode = "fresh"
	}
	if j.Timezone == "" {
		j.Timezone = "UTC"
	}
	if j.ConcurrencyPolicy == "" {
		j.ConcurrencyPolicy = "allow"
	}
	if j.CatchUp == "" {
		j.CatchUp = "skip"
	}
	return j
}

func cronJobConfig(schedule string, retries int) string {
	return fmt.Sprintf(`
resource "%[1]s_cron_job" "nightly" {
  app         = "web"
  name        = "nightly"
  schedule    = %[2]q
  command     = ["node", "scripts/nightly.js"]
  max_retries = %[3]d
}
`, TypeName, schedule, retries)
}

// TestAccCronJobLifecycle drives create, update, import and destroy through the real provider server.
func TestAccCronJobLifecycle(t *testing.T) {
	api := &fakeJobAPI{jobs: map[string]wireCronJob{}}
	srv := httptest.NewServer(http.HandlerFunc(api.handler))
	t.Cleanup(srv.Close)
	t.Setenv(EnvURL, srv.URL)
	t.Setenv(EnvToken, "tok")

	addr := TypeName + "_cron_job.nightly"
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories,
		CheckDestroy: func(*terraform.State) error {
			api.mu.Lock()
			defer api.mu.Unlock()
			if len(api.jobs) != 0 {
				return fmt.Errorf("jobs left after destroy: %d", len(api.jobs))
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: cronJobConfig("0 3 * * *", 0),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "id", "web/j1"),
					resource.TestCheckResourceAttr(addr, "mode", "fresh"),
					resource.TestCheckResourceAttr(addr, "timezone", "UTC"),
					resource.TestCheckResourceAttr(addr, "command.1", "scripts/nightly.js"),
				),
			},
			{
				Config: cronJobConfig("30 4 * * *", 2),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "schedule", "30 4 * * *"),
					resource.TestCheckResourceAttr(addr, "max_retries", "2"),
					resource.TestCheckResourceAttr(addr, "id", "web/j1"),
				),
			},
			{ResourceName: addr, ImportState: true, ImportStateId: "web/j1", ImportStateVerify: true},
		},
	})
}

func TestCronJobWireRoundTrip(t *testing.T) {
	m := cronJobModel{}
	m.fill("web", wireCronJob{ID: "j9", Name: "n", Command: []string{"a"}})
	if m.ID.ValueString() != "web/j9" || ruleIDOf(m.ID) != "j9" || m.wire().Command[0] != "a" {
		t.Fatalf("round trip lost data: %+v", m)
	}
}

const cronJobHTTPConfig = `
resource "%[1]s_cron_job" "warm" {
  app      = "web"
  name     = "warm"
  schedule = "*/10 * * * *"
  http = {
    path    = "/internal/warm"
    method  = "POST"
    headers = [{ name = "Authorization", secret_ref = "TOK", prefix = "Bearer " }]
  }
}

data "%[1]s_cron_jobs" "web" {
  app        = "web"
  depends_on = [%[1]s_cron_job.warm]
}
`

// TestAccCronJobHTTP drives an http job and the listing data source through the real provider server.
func TestAccCronJobHTTP(t *testing.T) {
	api := &fakeJobAPI{jobs: map[string]wireCronJob{}}
	srv := httptest.NewServer(http.HandlerFunc(api.handler))
	t.Cleanup(srv.Close)
	t.Setenv(EnvURL, srv.URL)
	t.Setenv(EnvToken, "tok")

	addr := TypeName + "_cron_job.warm"
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(cronJobHTTPConfig, TypeName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "mode", "http"),
					resource.TestCheckResourceAttr(addr, "http.method", "POST"),
					resource.TestCheckResourceAttr(addr, "http.headers.0.secret_ref", "TOK"),
					resource.TestCheckNoResourceAttr(addr, "command.0"),
					resource.TestCheckResourceAttr("data."+TypeName+"_cron_jobs.web", "jobs.0.http_path", "/internal/warm"),
				),
			},
			{ResourceName: addr, ImportState: true, ImportStateId: "web/j1", ImportStateVerify: true},
		},
	})
}
