package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// fakeStatusAPI serves the status page endpoints from memory.
func fakeStatusAPI(t *testing.T) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	page := wireStatusPage{}
	comps := map[string]wireStatusComponent{}
	var order []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		const base = "/api/v1/status-page"
		switch {
		case r.URL.Path == base && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(page)
		case r.URL.Path == base && r.Method == http.MethodPut:
			_ = json.NewDecoder(r.Body).Decode(&page)
			_ = json.NewEncoder(w).Encode(page)
		case r.URL.Path == base+"/components" && r.Method == http.MethodGet:
			out := []wireStatusComponent{}
			for _, id := range order {
				out = append(out, comps[id])
			}
			_ = json.NewEncoder(w).Encode(out)
		case r.URL.Path == base+"/components" && r.Method == http.MethodPost:
			var c wireStatusComponent
			_ = json.NewDecoder(r.Body).Decode(&c)
			c.ID = fmt.Sprintf("sc_%d", len(order)+1)
			comps[c.ID] = c
			order = append(order, c.ID)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(c)
		case strings.HasPrefix(r.URL.Path, base+"/components/"):
			id := strings.TrimPrefix(r.URL.Path, base+"/components/")
			if _, ok := comps[id]; !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if r.Method == http.MethodDelete {
				delete(comps, id)
				w.WriteHeader(http.StatusNoContent)
				return
			}
			var c wireStatusComponent
			_ = json.NewDecoder(r.Body).Decode(&c)
			c.ID = id
			comps[id] = c
			_ = json.NewEncoder(w).Encode(c)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func statusConfig(url, kind, name string, order int) string {
	return fmt.Sprintf(`
provider "%[1]s" {
  url   = %[2]q
  token = "tok"
}

resource "%[1]s_status_page" "page" {
  enabled = true
  title   = "Acme status"
}

resource "%[1]s_status_component" "web" {
  kind   = %[3]q
  target = "web"
  name   = %[4]q
  order  = %[5]d
}
`, TypeName, url, kind, name, order)
}

// Needs a terraform binary, so it only runs with TF_ACC set like the other plan tests.
func TestStatusPageResources(t *testing.T) {
	srv := fakeStatusAPI(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []resource.TestStep{
			{
				Config:      statusConfig(srv.URL, "bogus", "Web", 1),
				ExpectError: regexp.MustCompile(`kind must be one of app, domain, check`),
			},
			{
				Config: statusConfig(srv.URL, "app", "Web", 1),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(TypeName+"_status_page.page", "enabled", "true"),
					resource.TestCheckResourceAttr(TypeName+"_status_page.page", "custom_domain", ""),
					resource.TestCheckResourceAttr(TypeName+"_status_component.web", "name", "Web"),
					resource.TestCheckResourceAttrSet(TypeName+"_status_component.web", "id"),
				),
			},
			{
				Config: statusConfig(srv.URL, "app", "Storefront", 2),
				Check:  resource.TestCheckResourceAttr(TypeName+"_status_component.web", "name", "Storefront"),
			},
			{
				ResourceName:      TypeName + "_status_component.web",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
