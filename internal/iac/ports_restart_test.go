package iac

import (
	"strings"
	"testing"
)

const udpApp = `version: 1
kind: App
metadata: {name: dns}
spec:
  service:
    build: {type: image, image: "ghcr.io/acme/dns:1"}
    port: 8080
    restart: on-failure:3
    published_ports:
      - {container_port: 53, host_port: 53, protocol: udp}
      - {container_port: 53, host_port: 53}
`

func buildIssues(t *testing.T, src string) []Issue {
	t.Helper()
	docs, issues := ParseDocuments([]Source{{Name: "t.yaml", Data: []byte(src)}})
	if len(issues) == 0 {
		_, issues = Build(docs, Options{})
	}
	return issues
}

func TestAppPublishedPortsAndRestartRejectHostileValues(t *testing.T) {
	for name, tc := range map[string]struct{ from, to, want string }{
		"host port out of range":      {"host_port: 53, protocol: udp", "host_port: 70000, protocol: udp", "want 65,535"},
		"container port out of range": {"container_port: 53, host_port: 53, protocol: udp", "container_port: 0, host_port: 53, protocol: udp", "minimum: got 0"},
		"duplicate protocol and port": {"host_port: 53}", "host_port: 53, protocol: udp}", "declared twice"},
		"sctp":                        {"protocol: udp", "protocol: sctp", "'tcp', 'udp'"},
		"restart junk":                {"restart: on-failure:3", "restart: sometimes", `restart "sometimes"`},
		"restart negative count":      {"restart: on-failure:3", "restart: on-failure:-1", "on-failure:N"},
	} {
		t.Run(name, func(t *testing.T) {
			issues := buildIssues(t, strings.Replace(udpApp, tc.from, tc.to, 1))
			if len(issues) == 0 || !strings.Contains(issues[0].Message, tc.want) {
				t.Fatalf("issues = %v, want %q", issues, tc.want)
			}
		})
	}
}

func TestAppPublishedPortsAndRestartFlowThroughPlanAndApply(t *testing.T) {
	if issues := buildIssues(t, udpApp); len(issues) > 0 {
		t.Fatalf("valid ports and restart refused: %v", issues)
	}
	f := newFake()
	p := planFor(t, f, udpApp, Options{})
	var paths []string
	for _, c := range p.Changes {
		for _, fc := range c.Fields {
			paths = append(paths, fc.Path+"="+fc.New)
		}
	}
	joined := strings.Join(paths, " ")
	if !strings.Contains(joined, `publishedPorts=[{"container_port":53,"host_port":53,"protocol":"tcp"},{"container_port":53,"host_port":53,"protocol":"udp"}]`) || !strings.Contains(joined, "restart=on-failure:3") {
		t.Fatalf("plan fields = %s", joined)
	}
	applyAll(t, f, udpApp, Options{})
	app := f.apps["dns"]
	ports, _ := app["published_ports"].([]any)
	health, _ := app["health"].(map[string]any)
	if len(ports) != 2 || health["restart"] != "on-failure:3" {
		t.Fatalf("create body lost ports or restart: %v", app)
	}
	if p := planFor(t, f, udpApp, Options{}); p.Summary.Create+p.Summary.Update != 0 {
		t.Fatalf("replan not empty: %+v", p.Changes)
	}

	// An out-of-band port change is drift, and apply puts the declared list back.
	app["published_ports"] = []any{map[string]any{"container_port": 5353, "host_port": 5353, "protocol": "udp"}}
	p = planFor(t, f, udpApp, Options{})
	if p.Summary.Update != 1 || p.Changes[0].Fields[0].Path != "publishedPorts" {
		t.Fatalf("live port change not planned: %+v", p.Changes)
	}
	applyAll(t, f, udpApp, Options{})
	if ports, _ := f.apps["dns"]["published_ports"].([]any); len(ports) != 2 {
		t.Fatalf("apply did not restore ports: %v", f.apps["dns"]["published_ports"])
	}

	// A document that omits ports and restart leaves the live ones alone.
	bare := strings.Split(udpApp, "    restart:")[0]
	if p := planFor(t, f, bare, Options{}); p.Summary.Update != 0 {
		t.Fatalf("undeclared ports or restart planned: %+v", p.Changes)
	}
	cleared := strings.Split(udpApp, "    published_ports:")[0] + "    published_ports: []\n"
	applyAll(t, f, cleared, Options{})
	if ports, ok := f.apps["dns"]["published_ports"].([]any); !ok || len(ports) != 0 {
		t.Fatalf("an empty list did not clear the ports: %v", f.apps["dns"]["published_ports"])
	}
}

func exportAll(t *testing.T, f *fakeCP) string {
	t.Helper()
	res, err := Export(t.Context(), f, ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var parts []string
	for _, file := range res.Files {
		parts = append(parts, file.Content)
	}
	return strings.Join(parts, "---\n")
}

func TestExportKeepsPublishedPortsAndRestart(t *testing.T) {
	f := newFake()
	applyAll(t, f, udpApp, Options{})
	out := exportAll(t, f)
	if !strings.Contains(out, "protocol: udp") || !strings.Contains(out, "restart: on-failure:3") {
		t.Fatalf("export lost ports or restart:\n%s", out)
	}
	if issues := buildIssues(t, out); len(issues) > 0 {
		t.Fatalf("export does not rebuild: %v\n%s", issues, out)
	}
}

const udpOnlyApp = `version: 1
kind: App
metadata: {name: wg}
spec:
  service:
    build: {type: image, image: "ghcr.io/acme/wg:1"}
    published_ports:
      - {container_port: 51820, host_port: 51820, protocol: udp}
`

func TestUDPOnlyAppValidatesAppliesExportsAndRoundTrips(t *testing.T) {
	if issues := buildIssues(t, udpOnlyApp); len(issues) > 0 {
		t.Fatalf("udp-only app refused: %v", issues)
	}
	f := newFake()
	applyAll(t, f, udpOnlyApp, Options{})
	if p := planFor(t, f, udpOnlyApp, Options{}); p.Summary.Create+p.Summary.Update != 0 {
		t.Fatalf("replan not empty: %+v", p.Changes)
	}
	out := exportAll(t, f)
	if strings.Contains(out, "port: 0") || !strings.Contains(out, "protocol: udp") {
		t.Fatalf("export wrong:\n%s", out)
	}
	if issues := buildIssues(t, out); len(issues) > 0 {
		t.Fatalf("export does not rebuild: %v\n%s", issues, out)
	}
}

func TestAppPortRuleFailuresAreClear(t *testing.T) {
	withDomain := strings.Replace(udpOnlyApp, "    published_ports:", "    domains: [wg.example.com]\n    published_ports:", 1)
	noPorts := strings.Split(udpOnlyApp, "    published_ports:")[0]
	for name, tc := range map[string]struct{ src, want string }{
		"domain without port":  {withDomain, "port is required when domains are set"},
		"no port no published": {noPorts, "port is required unless"},
	} {
		t.Run(name, func(t *testing.T) {
			issues := buildIssues(t, tc.src)
			if len(issues) == 0 || !strings.Contains(issues[0].Message, tc.want) {
				t.Fatalf("issues = %v, want %q", issues, tc.want)
			}
		})
	}
}
