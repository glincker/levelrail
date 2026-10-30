# ADR 051: Read-only incident and cost analysis agent

Status: Proposed

Date: 2026-10-29

## Context

When an alert fires, an operator opens the alert, then logs, metrics,
the deploy timeline and diagnose output, and joins them by hand. Spend
alerts have the same shape: the numbers exist, the conclusion
("this app asks for 2 GiB and uses 300 MiB at p95") does not.

What exists today, read on `release/v0.8.11`:

- **Alerts.** The alerting engine (`internal/alerting/engine.go`) writes
  firings to alert history; the inbox (`internal/api/inbox.go`,
  `docs/alert-inbox.md`) lists warning and critical firings.
- **Cost.** `internal/usagecost` computes daily usage and allocated cost
  per app from measured CPU, memory, storage and egress
  (`types.go`, `collect.go`, `allocate.go`), budgets live in
  `budget.go` and `internal/store/cost_budgets.go`, spend caps alert at
  50, 80 and 100 percent (`internal/spendcap`, `internal/spendguard`,
  `docs/spend-caps.md`). Requested resources are
  `DesiredService.Resources`.
- **Deterministic diagnosis.** `internal/diagnose` has rule based causes
  for build, network and runtime failures.
- **AI plumbing.** The in-app assistant runs tools through `ToolCaller`
  (`internal/ai/toolcaller.go`), which speaks to the platform's own MCP
  tool surface in process over in-memory transports, and those tools call
  the REST API through `apiclient` like any external client. Each turn
  mints a short-lived agent token owned by the person who sent the
  message (`internal/ai/turn_credential.go`, `docs/ai-assistant.md`).
  Provider keys are user supplied, checked before storage, envelope
  encrypted, never returned or logged, with a fallback chain
  (`docs/ai-assistant-chat.md`).
- **Guardrails.** AI control modes `off`, `observe`, `operate`, `admin`
  are enforced server side for every agent token (`docs/ai-control.md`).
  Tools are classified read, mutating or destructive in one table
  (`internal/mcptools/classes.go`). Untrusted text from logs and deploy
  output is delimited, stripped and redacted before a model sees it
  (`docs/ai-assistant.md`, untrusted output section).

## Decision (recommended)

Build an analysis agent that is read-only by construction, off by
default, and never in the reconcile path.

**Triggers.** A warning or critical firing that reaches the inbox, a
spend cap threshold, or a budget alert enqueues an `analysis_runs` row.
The alerting engine only enqueues; it never waits on the agent. A manual
"Analyze" action on an alert or an app does the same. Runs are deduped
per alert fingerprint and per app inside `APP_ANALYSIS_DEDUP_WINDOW`.

**Identity.** Each run mints a token like the assistant turn token:
agent name `incident-analyst`, abilities `read` only (no
`read:sensitive`), TTL `APP_ANALYSIS_TOKEN_TTL`, revoked when the run
ends. The owner is the person who enabled the feature (recorded at
enable time) so IAM policies on that person bind the agent too. The run
requires AI control mode `observe` or higher; `off` means no run is
created.

**Tool access, three layers.**

1. Whitelist: an explicit list of read tool names (status, metrics,
   query logs, deploy timeline, diagnose, alert history, usage and cost,
   spend cap status). Not "all read tools".
2. Write guard: the run's `ToolCaller` wrapper refuses any tool whose
   class in `classes.go` is not `ClassRead`, even if the whitelist is
   edited wrongly.
3. Server: the token only holds `read` and the AI control mode is
   enforced by the API.

All reads go through the platform's own MCP surface. No direct store or
agent access from the analysis code.

**Deterministic digest first.** A new `internal/analysisdigest` package
calls the same whitelisted tools with the run token and produces:

- Metric stats per series over the alert window and a baseline window:
  min, max, mean, p50, p95, and change points; anomalies by a robust
  z score over `APP_ANALYSIS_ANOMALY_Z`.
- Grouped logs: lines fingerprinted by masking numbers, ids, hex and
  timestamps, with counts, first and last seen, and up to
  `APP_ANALYSIS_LOG_SAMPLES` sanitized samples per group.
- Deploy and config changes inside the window, and `diagnose` causes.
- For cost: requested vs actual (CPU and memory requests from
  `DesiredService.Resources`, actual p50 and p95 from metrics, daily cost
  from `usagecost`), and a right-sizing figure computed in code as p95
  times `APP_ANALYSIS_HEADROOM_PCT`, rounded to the next step, with the
  monthly cost delta from the configured prices.

Every item gets an evidence id. The model never computes a number.

**Model step.** The model receives the digest only (never raw logs or
raw series) and returns JSON: `root_causes[]` and `recommendations[]`,
each with a summary, a confidence label and `evidence_ids[]`. A
validator drops entries that cite unknown evidence ids and rejects
output that fails the schema. Numeric fields in the stored report are
copied from the digest, never from model text. The model uses the
existing provider configuration and fallback chain, with a per run token
budget `APP_ANALYSIS_MAX_TOKENS` and a monthly cap
`APP_ANALYSIS_MONTHLY_TOKENS`.

**Report.** Stored in `analysis_reports`: trigger, digest, model output,
validator notes, coverage (which nodes answered), tool call list, model
and provider used, cost in tokens. Viewable in the dashboard on the alert
detail and the app's cost panel, via `GET /api/v1/analysis/{id}`,
`levelrail-cli analysis show|list`, and a read MCP tool
`get_analysis_report`.

**Off by default.** `APP_EXPERIMENTAL=incident-analysis` plus a setting
that is disabled on a fresh instance, plus a configured provider key,
plus AI control at `observe` or higher. Any missing piece means no runs.

## Alternatives rejected

- **Reuse the chat assistant unchanged.** It is owned by a chat turn,
  sees raw tool output, and can request mutating tools behind a
  confirmation gate. An alert triggered job needs a narrower, unattended
  shape.
- **Let the model read raw telemetry.** Expensive, easy to inject, and
  the model would do arithmetic it gets wrong.
- **Direct store reads for speed.** Bypasses the audit trail, ability
  checks and AI control. Rejected on principle.
- **Auto remediation from the report.** Out of scope. Actions stay with
  people and the deterministic engine.

## Security

- Prompt injection through logs can at most bend the narrative: the
  token cannot write, the write guard refuses non read tools, and the
  validator forces citations.
- Telemetry leaves the instance only to the configured model provider,
  and only as the sanitized digest. The settings page states this and
  shows a preview of what a run would send.
- Every tool call is audited with `agent_name=incident-analyst`, the run
  id and `on_behalf_of` the enabling person. Refused calls are audited
  as AI control already does.
- No secret values: no `read:sensitive`, and existing redaction applies
  to samples.

## Failure modes

- Provider down or out of quota: the report stores the digest and marks
  the narrative unavailable. The digest alone is still useful.
- Node offline during a federated query: coverage lists missing nodes;
  the report says it is partial.
- Alert storm: dedup plus `APP_ANALYSIS_MAX_CONCURRENT` and
  `APP_ANALYSIS_MAX_RUNS_PER_HOUR`; excess runs are skipped and counted.
- Validator rejects the model output twice: report saved with digest
  only and the rejection reason.
- The enabling person loses access: runs stop with a clear reason until
  an admin re-enables.

## What cannot be verified locally

- Model quality of root causes on real incidents.
- Behavior during a real alert storm on a multi node instance.
- Provider latency and cost at production volumes.
- Federated query partial results with nodes really offline.

## Testing plan

- Digest: table tests for stats, z score anomalies, log fingerprinting
  and the right-sizing formula with env driven thresholds.
- Write guard: a mutating tool name in the whitelist is still refused.
- Validator: unknown evidence ids dropped; numbers in the report always
  equal digest values.
- Token: run token has `read` only, is revoked at the end, and is
  refused when AI control is `off`.
- One real run on a test server with a forced OOM and a forced spend cap
  alert.

## Phases

1. Cost digest only, no model, manual trigger.
2. Incident digest (metrics, grouped logs, deploy timeline, diagnose),
   manual trigger.
3. Model narrative with validator, still manual.
4. Automatic triggers from inbox firings and spend alerts, with dedup
   and rate limits.
5. Feedback (useful or not) on reports to tune thresholds.

## First thin slice (under 1 day)

Phase 1 for one app: `POST /api/v1/apps/{name}/analysis/cost` runs the
cost digest with a `read` agent token through `ToolCaller`, stores the
report, and returns requested vs actual CPU and memory, p95, the
right-sizing figure and the monthly delta. CLI `analysis cost APP`, a
read MCP tool, and a small dashboard panel on the app's cost tab render
it. No model call, behind the experimental flag.

Files: `internal/analysisdigest/cost.go`, `internal/store/analysis.go`
plus one migration, `internal/api/analysis.go`,
`cmd/levelrail-cli/analysis.go`, `internal/mcptools/tools_analysis.go`,
`web/src/features/analysis/*` and locale keys. Env:
`APP_ANALYSIS_HEADROOM_PCT`, `APP_ANALYSIS_WINDOW`.

## Slices and file ownership

1. Store and migration (`internal/store/analysis.go`). Sequential.
2. Cost digest and API, CLI, MCP, dashboard (thin slice above).
3. Incident digest (`internal/analysisdigest/metrics.go`, `logs.go`,
   `timeline.go`). Env: `APP_ANALYSIS_ANOMALY_Z`,
   `APP_ANALYSIS_LOG_SAMPLES`, `APP_ANALYSIS_BASELINE_WINDOW`.
4. Runner, token, write guard (`internal/analysis/runner.go`,
   `guard.go`). Env: `APP_ANALYSIS_TOKEN_TTL`,
   `APP_ANALYSIS_MAX_CONCURRENT`. Security sensitive, reviewed on its own.
5. Model step and validator (`internal/analysis/model.go`,
   `validate.go`). Env: `APP_ANALYSIS_MAX_TOKENS`,
   `APP_ANALYSIS_MONTHLY_TOKENS`.
6. Triggers (`internal/alerting` enqueue hook, spend cap and budget
   hooks). Env: `APP_ANALYSIS_DEDUP_WINDOW`,
   `APP_ANALYSIS_MAX_RUNS_PER_HOUR`.

## Risks

- Confident but wrong narratives erode trust. Mitigated by citations,
  confidence labels and always showing the digest first.
- Sending telemetry to a third party is a privacy decision some
  operators will not accept. Mitigated by off by default and a send
  preview; self-hosted providers already work.
- Token spend can surprise. Mitigated by per run and monthly caps.
- Scope creep toward remediation. This ADR does not allow it; that needs
  its own ADR.

## Consequences

- Alerts and spend alerts can carry a ready analysis without any change
  to reconciliation.
- One more agent identity and two more tables to operate.
- The deterministic digest is useful on its own, even with no model key.

## Open questions

- Who owns the run token when the enabling person leaves: a dedicated
  service principal, or require re-enable?
- Should `read:sensitive` ever be allowed (for example env key names)?
  Default no.
- Report retention period and whether reports count toward backups.
- Whether phase 4 should trigger on warning firings or critical only by
  default.
