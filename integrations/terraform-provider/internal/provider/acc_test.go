package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Acceptance tests run against a live control plane, see README.md. They
// are skipped unless TF_ACC=1 and both EnvURL and EnvToken are set.
func accPreCheck(t *testing.T) {
	t.Helper()
	if os.Getenv(EnvURL) == "" || os.Getenv(EnvToken) == "" {
		t.Skipf("%s and %s must point at a control plane", EnvURL, EnvToken)
	}
}

func TestAccProjectEnvironmentVariables(t *testing.T) {
	cfg := fmt.Sprintf(`
resource "%[1]s_project" "p" {
  name = "tfacc-project"
}

resource "%[1]s_environment" "staging" {
  project_id = %[1]s_project.p.id
  name       = "staging"
  protected  = false
}

resource "%[1]s_environment_variables" "shared" {
  scope    = "environment"
  scope_id = %[1]s_environment.staging.id
  variables = {
    LOG_LEVEL = "debug"
  }
}
`, TypeName)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(TypeName+"_environment.staging", "name", "staging"),
					resource.TestCheckResourceAttr(TypeName+"_environment_variables.shared", "variables.LOG_LEVEL", "debug"),
				),
			},
			{
				ResourceName:      TypeName + "_project.p",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccAppAndDomain(t *testing.T) {
	cfg := fmt.Sprintf(`
resource "%[1]s_app" "web" {
  name  = "tfacc-web"
  image = "nginx:alpine"
  port  = 80
}

resource "%[1]s_domain" "www" {
  app  = %[1]s_app.web.name
  host = "tfacc-web.example.test"
}

data "%[1]s_apps" "all" {
  depends_on = [%[1]s_domain.www]
}
`, TypeName)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(TypeName+"_domain.www", "id", "tfacc-web/tfacc-web.example.test"),
					resource.TestCheckResourceAttrSet("data."+TypeName+"_apps.all", "apps.#"),
				),
			},
			{ResourceName: TypeName + "_app.web", ImportState: true, ImportStateId: "tfacc-web", ImportStateVerify: true, ImportStateVerifyIdentifierAttribute: "name"},
			{ResourceName: TypeName + "_domain.www", ImportState: true, ImportStateId: "tfacc-web/tfacc-web.example.test", ImportStateVerify: true},
		},
	})
}
