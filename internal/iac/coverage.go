package iac

// CoverageStatus tells whether a product feature can be written in files.
type CoverageStatus string

// Coverage statuses of a matrix row.
const (
	CoverageInFiles CoverageStatus = "in_files"
	CoverageNotYet  CoverageStatus = "not_in_files_yet"
)

// CoverageRow is one feature of a kind. A not-yet row names the rejection a
// user sees when they try it; Probe is a minimal document that triggers it.
type CoverageRow struct {
	Kind    Kind
	Feature string
	Status  CoverageStatus
	// Code is the frozen machine code of the rejection, empty when the
	// rejection is a plain builder message identified by Reason.
	Code string
	// Reason is the rejection text a user sees (a substring of it).
	Reason string
	Probe  string
}

const (
	probeApp  = "version: 1\nkind: App\nmetadata: {name: a}\nspec:\n  service:\n    build: {type: image, image: x}\n    port: 80\n"
	unknownFn = "unknown field"
)

func appProbe(extra string) string { return probeApp + extra }

// coverageMatrix lists what each kind expresses and what the dashboard does
// that files cannot yet. Not-yet rows mirror the builder and schema
// rejections, so a claim here is also pinned by a probe in the test.
var coverageMatrix = []CoverageRow{
	{Kind: KindProject, Feature: "name and shared env", Status: CoverageInFiles},
	{Kind: KindEnvironment, Feature: "project, protected flag and env", Status: CoverageInFiles},
	{Kind: KindTag, Feature: "tag name", Status: CoverageInFiles},
	{Kind: KindNode, Feature: "labels and taints", Status: CoverageInFiles},
	{Kind: KindSetting, Feature: "saved runtime settings (non-sensitive)", Status: CoverageInFiles},
	{Kind: KindDatabase, Feature: "engine, version, project and pooler", Status: CoverageInFiles},
	{Kind: KindDatabase, Feature: "backup schedules", Status: CoverageNotYet, Code: CodeSchemaUnknownField, Reason: unknownFn,
		Probe: "version: 1\nkind: Database\nmetadata: {name: a}\nspec: {engine: postgres, version: \"16\", backup: {schedule: \"0 3 * * *\"}}\n"},
	{Kind: KindDatabase, Feature: "database placement", Status: CoverageNotYet, Code: CodeSchemaUnknownField, Reason: unknownFn,
		Probe: "version: 1\nkind: Database\nmetadata: {name: a}\nspec: {engine: postgres, version: \"16\", placement: {node: n1}}\n"},
	{Kind: KindApp, Feature: "image builds, env, ports, health, resources, replicas, rollout, files, jobs, volumes, placement", Status: CoverageInFiles},
	{Kind: KindApp, Feature: "git-source builds", Status: CoverageNotYet, Reason: "apply manages apps that run a prebuilt image",
		Probe: "version: 1\nkind: App\nmetadata: {name: a}\nspec:\n  service:\n    build: {type: dockerfile}\n    port: 80\n"},
	{Kind: KindApp, Feature: "egress policies", Status: CoverageNotYet, Reason: "egress policies are not supported by apply yet",
		Probe: appProbe("    egress: {mode: allowlist}\n")},
	{Kind: KindApp, Feature: "GPU requests", Status: CoverageNotYet, Reason: "GPU requests are not supported by apply yet",
		Probe: appProbe("    resources: {gpu: {count: 1}}\n")},
	{Kind: KindApp, Feature: "registry credentials", Status: CoverageNotYet, Reason: "registry credentials are not supported by apply yet",
		Probe: "version: 1\nkind: App\nmetadata: {name: a}\nspec:\n  service:\n    build: {type: image, image: x, registryCredential: reg}\n    port: 80\n"},
	{Kind: KindApp, Feature: "vault env references", Status: CoverageNotYet, Reason: "vault references are not supported by apply yet",
		Probe: appProbe("    env: {K: {vault: {path: kv/a, key: k}}}\n")},
	{Kind: KindApp, Feature: "host-path volumes", Status: CoverageNotYet, Reason: "named volumes only",
		Probe: appProbe("    volumes: [{hostPath: /srv/data, path: /data}]\n")},
	{Kind: KindApp, Feature: "pre-deploy snapshot policy", Status: CoverageNotYet, Code: CodeSchemaUnknownField, Reason: unknownFn,
		Probe: "version: 1\nkind: App\nmetadata: {name: a}\nspec:\n  preDeploySnapshot: true\n  service:\n    build: {type: image, image: x}\n    port: 80\n"},
	{Kind: KindDomain, Feature: "hostname attached to an app", Status: CoverageInFiles},
	{Kind: KindDomain, Feature: "TLS settings", Status: CoverageNotYet, Code: CodeSchemaUnknownField, Reason: unknownFn,
		Probe: "version: 1\nkind: Domain\nmetadata: {name: a.example.com}\nspec: {app: web, tls: {mode: auto}}\n"},
	{Kind: KindDomain, Feature: "redirects", Status: CoverageNotYet, Code: CodeSchemaUnknownField, Reason: unknownFn,
		Probe: "version: 1\nkind: Domain\nmetadata: {name: a.example.com}\nspec: {app: web, redirect: {to: b.example.com}}\n"},
	{Kind: KindDomain, Feature: "response headers", Status: CoverageNotYet, Code: CodeSchemaUnknownField, Reason: unknownFn,
		Probe: "version: 1\nkind: Domain\nmetadata: {name: a.example.com}\nspec: {app: web, headers: {X-A: b}}\n"},
	{Kind: KindLoadBalancer, Feature: "backends and balancing (beta)", Status: CoverageInFiles},
	{Kind: KindPipeline, Feature: "pipeline definition (beta)", Status: CoverageInFiles},
	{Kind: KindAlertRule, Feature: "threshold rules (beta)", Status: CoverageInFiles},
}

// CoverageMatrix returns a copy of the matrix, ordered by kind then row.
func CoverageMatrix() []CoverageRow {
	out := make([]CoverageRow, len(coverageMatrix))
	copy(out, coverageMatrix)
	return out
}
