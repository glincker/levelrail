// Command catalog-smoke prepares and reports the local template smoke run.
//
//	go run ./scripts/catalog-smoke prepare -top 50 -out DIR
//	go run ./scripts/catalog-smoke report -results DIR/results.tsv -out docs/templates-status.md
//
// prepare writes one runnable compose file per template (magic variables
// resolved, host ports dropped) plus DIR/index.tsv. scripts/catalog-smoke.sh
// drives docker compose over that index and writes results.tsv.
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/compose"
)

//go:embed notes.md
var notes string

var priority = []string{
	"ghost", "wordpress", "nextcloud", "grafana", "uptime-kuma", "n8n", "plausible", "metabase",
	"vaultwarden", "gitea", "minio", "portainer", "nocodb", "directus", "strapi", "redis", "postgres",
	"mysql", "mariadb", "mongodb", "adminer", "pgadmin", "umami", "penpot", "jellyfin", "immich",
	"paperless-ngx", "bookstack", "outline", "appsmith", "meilisearch", "typesense", "sonarqube",
	"prometheus", "open-webui", "searxng", "homepage", "dozzle", "filebrowser", "memos", "linkding",
	"changedetection", "pocketbase", "mattermost", "rocketchat", "listmonk", "it-tools", "whoami",
	"nginx", "caddy", "traefik", "keycloak", "authentik", "forgejo", "drone", "woodpecker", "gotify",
	"ntfy", "mealie", "actual-budget", "firefly-iii", "freshrss", "miniflux", "wikijs", "docuseal",
}

func main() {
	if len(os.Args) < 2 {
		fatal(fmt.Errorf("usage: catalog-smoke prepare|report [flags]"))
	}
	switch os.Args[1] {
	case "prepare":
		fatal(prepare(os.Args[2:]))
	case "report":
		fatal(report(os.Args[2:]))
	default:
		fatal(fmt.Errorf("unknown subcommand %q", os.Args[1]))
	}
}

func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "catalog-smoke:", err)
		os.Exit(1)
	}
}

type candidate struct {
	tpl    catalog.Template
	reason string
}

func prepare(args []string) error {
	fs := flag.NewFlagSet("prepare", flag.ExitOnError)
	top := fs.Int("top", 50, "number of templates to select")
	out := fs.String("out", "", "output directory")
	ids := fs.String("ids", "", "comma-separated template IDs (overrides -top selection)")
	_ = fs.Parse(args)
	if *out == "" {
		return fmt.Errorf("-out is required")
	}
	if err := os.MkdirAll(*out, 0o750); err != nil {
		return err
	}

	var chosen []catalog.Template
	if *ids != "" {
		for _, id := range strings.Split(*ids, ",") {
			tpl, ok := catalog.TemplateByID(strings.TrimSpace(id))
			if !ok {
				return fmt.Errorf("unknown template %q", id)
			}
			chosen = append(chosen, tpl)
		}
	} else {
		chosen = selectTop(*top)
	}

	var index, skipped strings.Builder
	for _, tpl := range chosen {
		body, reason := render(tpl)
		if reason != "" {
			fmt.Fprintf(&skipped, "%s\t%s\n", tpl.ID, reason)
			continue
		}
		dir := filepath.Join(*out, tpl.ID)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return err
		}
		file := filepath.Join(dir, "compose.yaml")
		if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
			return err
		}
		fmt.Fprintf(&index, "%s\t%s\t%s\n", tpl.ID, tpl.Name, file)
	}
	if err := os.WriteFile(filepath.Join(*out, "index.tsv"), []byte(index.String()), 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(*out, "skipped.tsv"), []byte(skipped.String()), 0o600)
}

func selectTop(n int) []catalog.Template {
	seen := map[string]bool{}
	var out []catalog.Template
	add := func(tpl catalog.Template) {
		if len(out) >= n || seen[tpl.ID] || tpl.RequiresGPU {
			return
		}
		if _, reason := render(tpl); reason != "" {
			return
		}
		seen[tpl.ID] = true
		out = append(out, tpl)
	}
	for _, id := range priority {
		if tpl, ok := catalog.TemplateByID(id); ok {
			add(tpl)
		}
	}
	for _, tpl := range catalog.Templates {
		add(tpl)
	}
	return out
}

var tokenRe = regexp.MustCompile(`\$\{SERVICE_[A-Za-z0-9_]+?(?::-[^}]*)?\}|\$SERVICE_[A-Za-z0-9_]+`)

// render returns a self-contained compose body, or a skip reason.
func render(tpl catalog.Template) (string, string) {
	generated := map[string]string{}
	var unresolved string
	body := tokenRe.ReplaceAllStringFunc(tpl.Compose, func(tok string) string {
		mv := compose.FindMagicVars(tok)[0]
		cacheKey := mv.Kind + "|" + mv.Key + "|" + fmt.Sprint(mv.Length)
		if v, ok := generated[cacheKey]; ok {
			return v
		}
		var val string
		switch {
		case mv.Generatable:
			v, err := compose.GenerateValue(mv.Kind, mv.Length)
			if err != nil {
				unresolved = err.Error()
				return tok
			}
			val = v
		case mv.Kind == "FQDN", mv.Kind == "URL":
			// The platform resolves FQDN to a full URL, so the harness must too.
			val = "http://" + tpl.ID + ".localhost"
		case mv.HasDefault:
			val = mv.Default
		default:
			unresolved = "needs configuration: " + tok
			return tok
		}
		generated[cacheKey] = val
		return val
	})
	if unresolved != "" {
		return "", unresolved
	}
	stripped, err := dropHostPorts(body, noInContainerProbe[tpl.ID])
	if err != nil {
		return "", "unparseable compose: " + err.Error()
	}
	return stripped, ""
}

// noInContainerProbe lists images with no shell, curl or wget. The platform probes
// them over HTTP from outside, which docker compose cannot, so the harness only
// checks that they stay running.
var noInContainerProbe = map[string]bool{"portainer": true, "keycloak": true, "typesense": true, "shiori": true, "vikunja": true, "loki": true}

// dropHostPorts removes services.*.ports so a smoke run never collides with local ports.
func dropHostPorts(body string, dropHealth bool) (string, error) {
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		return "", err
	}
	named := map[string]any{}
	if top, ok := doc["volumes"].(map[string]any); ok {
		named = top
	}
	if svcs, ok := doc["services"].(map[string]any); ok {
		for _, raw := range svcs {
			svc, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			delete(svc, "ports")
			escapeContainerVars(svc)
			// Only HTTP probes are dropped: a database healthcheck still gates depends_on.
			if hc, ok := svc["healthcheck"].(map[string]any); ok && dropHealth && strings.Contains(fmt.Sprint(hc["test"]), "http") {
				delete(svc, "healthcheck")
			}
			vols, _ := svc["volumes"].([]any)
			for _, v := range vols {
				if name := namedVolume(v); name != "" {
					if _, declared := named[name]; !declared {
						named[name] = map[string]any{}
					}
				}
			}
		}
	}
	if len(named) > 0 {
		doc["volumes"] = named
	}
	out, err := yaml.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// escapeContainerVars rewrites $KEY to $$KEY in command, entrypoint and healthcheck
// for keys the service declares in environment. The platform leaves those to the
// container shell, but docker compose would expand them from the host (blank).
func escapeContainerVars(svc map[string]any) {
	env, _ := svc["environment"].(map[string]any)
	if len(env) == 0 {
		return
	}
	names := make([]string, 0, len(env))
	for k := range env {
		names = append(names, regexp.QuoteMeta(k))
	}
	re := regexp.MustCompile(`(^|[^$])\$(\{(?:` + strings.Join(names, "|") + `)\}|(?:` + strings.Join(names, "|") + `)\b)`)
	esc := func(s string) string { return re.ReplaceAllString(s, "${1}$$$$${2}") }
	var walk func(v any) any
	walk = func(v any) any {
		switch t := v.(type) {
		case string:
			return esc(t)
		case []any:
			for i := range t {
				t[i] = walk(t[i])
			}
		}
		return v
	}
	for _, k := range []string{"command", "entrypoint"} {
		if v, ok := svc[k]; ok {
			svc[k] = walk(v)
		}
	}
	if hc, ok := svc["healthcheck"].(map[string]any); ok {
		if v, ok := hc["test"]; ok {
			hc["test"] = walk(v)
		}
	}
}

// namedVolume returns the volume name when v mounts a named volume, since
// the platform accepts undeclared ones but docker compose does not.
func namedVolume(v any) string {
	switch m := v.(type) {
	case string:
		src, _, ok := strings.Cut(m, ":")
		if ok && src != "" && !strings.ContainsAny(src[:1], "./~$") && !strings.Contains(src, "/") {
			return src
		}
	case map[string]any:
		if m["type"] == "volume" {
			if src, ok := m["source"].(string); ok && !strings.Contains(src, "$") {
				return src
			}
		}
	}
	return ""
}

type result struct {
	id, status, seconds, note string
}

func report(args []string) error {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	results := fs.String("results", "", "results.tsv written by the catalog smoke script")
	out := fs.String("out", "docs/templates-status.md", "markdown file to write")
	_ = fs.Parse(args)
	raw, err := os.ReadFile(*results)
	if err != nil {
		return err
	}
	var rows []result
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		f := strings.SplitN(line, "\t", 4)
		if len(f) < 3 {
			continue
		}
		r := result{id: f[0], status: f[1], seconds: f[2]}
		if len(f) == 4 {
			r.note = strings.ReplaceAll(f[3], "|", "/")
		}
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].id < rows[j].id })
	pass := 0
	for _, r := range rows {
		if r.status == "pass" {
			pass++
		}
	}
	var b strings.Builder
	b.WriteString("---\ntitle: \"Template status\"\ndescription: Measured pass rate of the one-click template catalog from the local smoke run\n\"og:url\": \"https://docs.levelrail.com/templates-status\"\n---\n\n")
	b.WriteString("Measured by the catalog smoke script (scripts/catalog-smoke.sh): each template's compose file is brought up with `docker compose up --wait`, checked for running and healthy containers, then torn down with its volumes. Host ports are dropped so runs never collide. Regenerate with `scripts/catalog-smoke.sh -r` (all of the top 50) or pass `-i id1,id2` for a subset. A fail with an unhealthy container means the template's own healthcheck never passed on this host, so check that healthcheck and the image first.\n\n")
	rate := 0.0
	if len(rows) > 0 {
		rate = float64(pass) * 100 / float64(len(rows))
	}
	fmt.Fprintf(&b, "Last run: %s. **%d of %d passed (%.0f%%).** Templates that need operator configuration are skipped and listed in the run output, not counted.\n\n", time.Now().UTC().Format("2006-01-02"), pass, len(rows), rate)
	b.WriteString("| Template | Result | Seconds | Note |\n| --- | --- | --- | --- |\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", r.id, r.status, r.seconds, r.note)
	}
	b.WriteString("\n")
	b.WriteString(notes)
	return os.WriteFile(*out, []byte(b.String()), 0o600)
}
