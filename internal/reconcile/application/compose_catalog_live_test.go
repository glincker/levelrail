package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/compose"
)

// liveCatalogSample is the fixed set of imported templates installed on
// real Docker. It spans databases (Postgres, MariaDB, MySQL, Redis),
// single-binary apps and multi-service stacks, picked for small images.
// Templates whose images exceed the size cap (flowise-with-databases,
// hermes-agent-with-webui, mage-ai, metamcp, mindsdb, paperless) are
// committed but not in the sample.
var liveCatalogSample = []string{
	"bento-pdf", "cockpit", "gitea-with-postgresql", "gitea-with-mariadb", "forgejo-with-postgresql",
	"freshrss-with-postgresql", "grafana-with-postgresql", "wordpress-with-mariadb", "wordpress-with-mysql",
	"drupal-with-postgresql", "uptime-kuma-with-mariadb", "docuseal-with-postgres", "codimd",
	"supertokens-with-postgresql", "pocket-id-with-postgresql", "joomla-with-mariadb",
	"classicpress-with-mariadb", "vvveb-with-mariadb", "next-image-transformation",
	"nitropage-with-postgresql", "yamtrack-with-postgresql", "unleash-with-postgresql",
	"n8n-with-postgresql", "convex", "labelstudio", "evolution-api", "newapi", "plunk", "cap", "executor",
}

// defaultImageCapMB bounds the layers one image may download; override
// with LEVELRAIL_LIVE_IMAGE_CAP_MB.
const defaultImageCapMB = 900

var errImageTooLarge = errors.New("image exceeds the live-test size cap")

// pullCapped pulls ref unless it is already present, aborting once the
// layers the daemon announces add up past capBytes. pulled reports
// whether this call fetched it, so the caller can remove it afterwards.
func pullCapped(ctx context.Context, cli *dockerclient.Client, ref string, capBytes int64) (pulled bool, err error) {
	if _, ierr := cli.ImageInspect(ctx, ref); ierr == nil {
		return false, nil
	} else if !cerrdefs.IsNotFound(ierr) {
		return false, fmt.Errorf("inspect %s: %w", ref, ierr)
	}
	pctx, cancel := context.WithCancel(ctx)
	defer cancel()
	rc, err := cli.ImagePull(pctx, ref, image.PullOptions{})
	if err != nil {
		return false, fmt.Errorf("pull %s: %w", ref, err)
	}
	defer func() { _ = rc.Close() }()
	layers := map[string]int64{}
	dec := json.NewDecoder(rc)
	for {
		var msg struct {
			ID             string `json:"id"`
			Error          string `json:"error"`
			ProgressDetail struct {
				Total int64 `json:"total"`
			} `json:"progressDetail"`
		}
		if derr := dec.Decode(&msg); derr != nil {
			if errors.Is(derr, io.EOF) {
				return true, nil
			}
			return true, fmt.Errorf("pull %s: %w", ref, derr)
		}
		if msg.Error != "" {
			return true, fmt.Errorf("pull %s: %s", ref, msg.Error)
		}
		if msg.ID != "" && msg.ProgressDetail.Total > 0 {
			layers[msg.ID] = msg.ProgressDetail.Total
		}
		var sum int64
		for _, n := range layers {
			sum += n
		}
		if sum > capBytes {
			cancel()
			return true, fmt.Errorf("%w: %s needs more than %d MB", errImageTooLarge, ref, capBytes>>20)
		}
	}
}

// liveTemplates is the fixed sample, or the comma-separated ids in
// LEVELRAIL_LIVE_TEMPLATES when exploring other templates by hand.
func liveTemplates() []string {
	if v := strings.TrimSpace(os.Getenv("LEVELRAIL_LIVE_TEMPLATES")); v != "" {
		return strings.Split(v, ",")
	}
	return liveCatalogSample
}

// liveConvergeBudget bounds how long one stack may take to become Ready;
// override with LEVELRAIL_LIVE_CONVERGE_SEC.
func liveConvergeBudget() time.Duration {
	if n, err := strconv.Atoi(os.Getenv("LEVELRAIL_LIVE_CONVERGE_SEC")); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return 5 * time.Minute
}

func liveImageCap() int64 {
	mb := int64(defaultImageCapMB)
	if v, err := strconv.ParseInt(os.Getenv("LEVELRAIL_LIVE_IMAGE_CAP_MB"), 10, 64); err == nil && v > 0 {
		mb = v
	}
	return mb << 20
}

// liveInstallInput fills every required prompt and offers each domain,
// the way an operator completing the install form would.
func liveInstallInput(m *catalog.Manifest) catalog.InstallInput {
	in := catalog.InstallInput{Env: map[string]string{}, Domains: map[string]string{}}
	for _, p := range m.Env {
		if p.Required && p.Default == "" && p.Generate == "" {
			in.Env[p.Key] = "live-test"
		}
	}
	for _, d := range m.Domains {
		in.Domains[d.Service] = d.Service + ".example.com"
	}
	return in
}

// TestComposeCatalogSample_Live installs the fixed sample of imported
// templates on real Docker one at a time: every service must converge to
// Ready (healthchecks passing), the web healthcheck must answer, and the
// stack plus any image this test pulled is removed afterwards. Images over
// the size cap are skipped and named in the test output.
func TestComposeCatalogSample_Live(t *testing.T) {
	rt := liveDocker(t)
	raw, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	capBytes := liveImageCap()

	for _, id := range liveTemplates() {
		t.Run(id, func(t *testing.T) {
			tpl, ok := catalog.TemplateByID(id)
			if !ok || tpl.Manifest == nil {
				t.Fatalf("template %q is not a manifest in the catalog", id)
			}
			m := tpl.Manifest
			body, err := m.Render(liveInstallInput(m), false)
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			f, err := compose.Parse([]byte(body))
			if err != nil {
				t.Fatalf("compose.Parse() error = %v", err)
			}
			ctx := context.Background()
			var pulled []string
			t.Cleanup(func() {
				for _, ref := range pulled {
					_, _ = raw.ImageRemove(ctx, ref, image.RemoveOptions{PruneChildren: true})
				}
			})
			for _, svc := range f.Services {
				didPull, perr := pullCapped(ctx, raw, svc.Image, capBytes)
				if didPull {
					pulled = append(pulled, svc.Image)
				}
				if errors.Is(perr, errImageTooLarge) {
					t.Skipf("skipped, heavy image: %v", perr)
				}
				if perr != nil {
					t.Fatalf("%v", perr)
				}
			}
			s := deployLiveStack(t, rt, "lrcat-"+id, body)
			s.converge(t, liveConvergeBudget())
			for _, c := range s.ctrls {
				key := strings.TrimPrefix(c.serviceName, s.app+"-")
				if s.container(t, key) == nil {
					t.Errorf("service %s has no running container after convergence", key)
				}
			}
			if m.Healthcheck == nil {
				return
			}
			addr, err := PrimaryAddr(s.container(t, m.Healthcheck.Service))
			if err != nil {
				t.Fatalf("PrimaryAddr() error = %v", err)
			}
			resp, err := http.Get(fmt.Sprintf("http://%s%s", addr, m.Healthcheck.Path)) //nolint:gosec,noctx // test-only loopback probe
			if err != nil {
				t.Fatalf("GET healthcheck: %v", err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode >= 500 {
				t.Fatalf("healthcheck %s answered %d", m.Healthcheck.Path, resp.StatusCode)
			}
		})
	}
}
