---
description: Per-feature maturity labels (stable, beta, hide-behind-flag) with the test, e2e, docs and real-infrastructure evidence behind each one.
---

# Feature status

This page labels every major feature by how much evidence backs it today. The evidence was gathered on 2026-09-26. Counts and the template and MCP rows were re-verified against main at `2622220e` on 2026-10-04. Every claim cites a path. Where evidence is absent, the page says so instead of guessing.

## How to read the labels

| Label | Meaning |
| --- | --- |
| **stable** | Unit tests, a doc page, and either a live end-to-end test in `test/e2e` or a documented live verification. |
| **beta** | Works and is tested, but has no end-to-end test and no record of a run on real infrastructure. Expect rough edges. |
| **hide-behind-flag** | Large or hardware-dependent surface with no end-to-end or real-infrastructure evidence, or outside the core single-node story. Should be off unless an operator opts in. |

Evidence tiers used below, weakest to strongest:

1. **Unit**: `*_test.go` against fakes or `httptest` servers.
2. **Live Docker**: tests named `*_Live_*` or `*_live_test.go` that skip when no Docker daemon is reachable and run against a real local daemon (for example `internal/pipeline/live_test.go`, `internal/backup/pitr_live_test.go`). `nightly.yml` has a `docker` lane, but `test/e2e` itself is only named in the flake sweep (`.github/workflows/nightly.yml:111`).
3. **E2E**: `test/e2e/*_test.go`, which drive the real HTTP API and a real reconciler against real containers on one machine.
4. **Real infrastructure**: a fresh VPS, a real public domain, real vendor endpoints. **Almost no feature has this evidence in the repository**; multi-node enrollment is the one documented exception so far, verified locally across two real Docker daemons (see its own section below), not against a real VPS or a real WAN. `docs/roadmap.md` ("In progress") states real public ACME was never verified against a live domain, and its e2e note says the suite "does not yet exercise a full multi-node mesh or real ACME against a live domain". `docs/acme-verification-runbook.md` exists but has no recorded run.

Because tier 4 is empty for almost every row, the "real infra" column below reads "none found" outside multi-node. The distinction that matters is tier 3 versus tiers 1 and 2.

## Summary

| Area | Label | One-line justification |
| --- | --- | --- |
| Deploy approvals | stable | Live e2e proves a second user's approval is the only thing that cuts over (`test/e2e/protected_environment_test.go`). |
| PITR and database backups | stable | Live e2e restores before a marker row through the real API with MinIO (`test/e2e/pitr_test.go`), plus live Docker dump and restore tests. |
| Previews | stable (GitHub), beta (GitLab, Bitbucket) | Lifecycle e2e exists for all three (`test/e2e/preview_environments_*_test.go`), but they replay synthetic webhooks, not a real forge. Kept beta for the two forges with less usage. |
| IAM | beta | Broadest unit coverage in the repo, but the only e2e is the auth session lifecycle, not policy enforcement. |
| Notification channels (18) | beta | All 18 kinds have unit tests against `httptest`, none against a real vendor. |
| Pipelines | beta | Live Docker test exists (`internal/pipeline/live_test.go`), no e2e. |
| Deploy freeze | beta | Unit, API and CLI tests, documented in `docs/deploy-safety.md`, no e2e. |
| Feature flags | beta | Unit, API, CLI, MCP tests and a doc page, no e2e. |
| Templates | beta | 311 catalog entries. Unit tests check shape and a floor of 180, and a live e2e deploys a sample of 16, not the whole catalog. |
| Log archive | beta | Unit and e2e tests (`test/e2e/log_archive_test.go`), dedicated doc page (`docs/log-archive.md`). |
| Supply chain | beta | Off by default (`docs/supply-chain.md`), unit tests, no e2e. |
| Status page | beta | Off by default, one internal package test file plus API tests, no e2e. |
| Multi-node and WireGuard | beta | Join flow verified locally across two real Docker daemons (enrollment, cordon, drain with real container relocation); the WireGuard mesh itself and cross-host remote transport are still unverified. |
| AI assistant (MCP server) | beta | 156 tools in full mode (146 in the default standard mode, 118 read-only), 31 test files in `internal/mcptools`, no e2e. |
| AI assistant (in-app chat) | hide-behind-flag | Tested against fake Anthropic responses only (unit, plus a real-HTTP e2e session lifecycle test with a fake provider); `docs/ai-assistant-chat.md` now documents it. |
| AI models (GPU) | hide-behind-flag | Needs NVIDIA hardware, every test uses fakes, docs state v1 scope is NVIDIA on Linux only. |
| Load balancer | hide-behind-flag | No e2e, no live test, outside the 3 to 50 services story in the project plan. |
| Platform as code (iac) | hide-behind-flag | Widest surface relative to tests: 17 source files, 4 test files, no e2e. |
| Cloudflare tunnel | hide-behind-flag | Reconciler unit tests only, needs a real Cloudflare account, no dedicated doc page. |

## Evidence per area

Test counts are `*_test.go` files in the named directory. "CLI" and "web" list test files found in `cmd/levelrail-cli` and `web/src`.

### AI assistant (MCP server and in-app chat)

- Unit: `internal/ai/` (5 files: `anthropic_test.go`, `classify_test.go`, `engine_test.go`, `eval_injection_test.go`, `toolcaller_test.go`); `internal/mcptools/` (31 files, for example `modes_test.go`, `tools_apps_clone_images_test.go`); API `internal/api/ai_chat_test.go`, `internal/api/ai_settings_test.go`; CLI `cmd/levelrail-cli/settings_ai_assistant_test.go`; web `web/src/components/AiChatMessageList.test.tsx`, `AiToolCallCard.test.tsx`, `web/src/queries/aiAssistant.test.ts`, `web/src/components/AiChatHistoryMenu.test.tsx`.
- E2E: `test/e2e/ai_chat_test.go` drives create/send-message/get/list/delete over real HTTP against a real `*api.Router` and `ai.Engine`, with a fake `ai.Provider` and `ai.ToolExecutor` (no live Anthropic call). MCP server itself: none.
- Docs: `docs/ai-assistant.md` covers `levelrail-mcp`; `docs/ai-assistant-chat.md` covers the in-app chat.
- Real infra: none found.
- Label: MCP server **beta** (tool registry is well tested, but the surface is large and unexercised end to end). In-app chat **hide-behind-flag** (depends on an external model provider, no evidence beyond fakes).

### AI models (GPU inference)

- Unit: `internal/models/` (13 files, for example `gpunodes_test.go`, `engines_test.go`, `gateway_test.go`, `prober_test.go`); API `internal/api/models_test.go`, `models_hf_test.go`, `models_keys_test.go`; CLI `models_test.go`, `models_keys_test.go`, `models_preflight_test.go`; web `ModelRow.test.tsx`, `ModelCacheCard.test.tsx`, `ModelKeysPanel.test.tsx`, `ModelPreflightPanel.test.tsx`.
- E2E: none. No live Docker test either.
- Docs: `docs/ai-models.md` (264 lines). It states v1 supports NVIDIA GPUs on Linux only.
- Real infra: none found. No test touches a real GPU.
- Label: **hide-behind-flag**. Correctness depends on driver, container toolkit and VRAM behavior that no test in the repo observes.

### Load balancer

- Unit: `internal/loadbalancer/` (4 files: `check_test.go`, `config_test.go`, `history_test.go`, `status_test.go`); API `loadbalancer_test.go`, `loadbalancer_history_test.go`, `loadbalancer_list_test.go`; CLI `lb_test.go`, `lb_history_test.go`, `lb_list_test.go`; web `LoadBalancerOverview.test.tsx`, `web/src/lib/loadBalancer.test.ts`; MCP `tools_loadbalancer_test.go` and two more.
- E2E: none. Live Docker: none.
- Docs: `docs/load-balancing.md` (162 lines).
- Real infra: none found.
- Label: **hide-behind-flag**. Multi-replica routing is built on embedded Caddy, and nothing verifies routing across real replicas.

### Pipelines

- Unit: `internal/pipeline/` (14 files, including `engine_test.go`, `definition_test.go`, `trigger_event_test.go`, `pathtrigger_test.go`, `sync_test.go`); API `pipelines_test.go`, `pipelines_events_ci_test.go`, `pipelines_iam_test.go`, `pipelines_sync_test.go`; CLI `pipelines_test.go`, `pipelines_all_test.go`, `pipelines_sync_test.go`; web has six `Pipeline*.test.tsx` files.
- Live Docker: `internal/pipeline/live_test.go` runs a two-job pipeline with artifact handoff and a failing step, then checks nothing leaks.
- E2E: none. Matches for "pipeline" in `test/e2e` are the deploy pipeline type (`internal/deploy.Pipeline`), not this feature.
- Docs: `docs/pipelines.md` (369 lines).
- Real infra: none found.
- Label: **beta**. Better evidenced than most, but `test/e2e` does not cover it.

### Notification channels (all 18)

Kinds, from `internal/alerting/rules.go:87-104`: generic, slack, discord, telegram, email, pushover, pagerduty, teams, resend, ntfy, gotify, mattermost, lark, rocketchat, opsgenie, webex, googlechat, webpush. That is 18, matching the README and comparison count. `webpush` (browser push, `internal/webpush/`) is the newest and the only kind whose destination isn't an operator-supplied URL: it fans out to every registered browser subscription instead.

- Unit: `internal/alerting/` (28 files). Payload tests live in `notify_test.go` (`TestNotifyTelegram_PostsChatIDAndText`, `TestNotifyResend_PostsAuthHeaderAndPayload`, `TestNotifyOpsgenie_PostsAuthHeaderAndPayload`) and `TestNewNotifier_AllValidKinds_Recognized` (`notify_test.go:1095`). That table test covers a subset of the kinds, so the rest rely on their own payload tests, and coverage is thinnest for pushover, pagerduty and teams.
- API `notification_channels_test.go`; CLI: no dedicated channel test file found; email retry tests at `notify_test.go:569-629`.
- E2E: none. Every test posts to an `httptest` server.
- Docs: covered in `docs/integrations.md` and `docs/observability.md`. `docs/integrations.md` is 45 lines.
- Real infra: none found. No test sends to a real Slack, PagerDuty, SMTP or other vendor.
- Label: **beta**. Suggest a manual send-test pass per vendor before calling any of them stable.

### Multi-node and WireGuard mesh

- Unit: `internal/network/` (13 files), `internal/agent/` (about 20 files including `grpc_transport_test.go`, `pki_test.go`, `renew_test.go`, `mesh_dispatch_test.go`), `cmd/levelrail/mesh_test.go` (skips without Docker); CLI `nodes_*_test.go` (about 14 files); reconcile `internal/reconcile/mesh/controller_test.go`.
- Live Docker: `internal/agent/live_test.go`, `internal/agent/build_live_test.go`, `internal/reconcile/application/placement_live_test.go`.
- E2E: `test/e2e/node_placement_test.go`. Its own header says one Docker daemon cannot exercise the NodeID-to-remote-agent resolution and that it only proves each controller uses the runtime it was built with.
- Docs: `docs/multi-node.md` (542 lines).
- Real infra: the join flow was verified against two real agent processes, each with its own real Docker daemon (separate `docker:27-dind` containers), running against a real control plane binary. Enrollment, cordon, drain with actual container relocation between the two daemons, and explicit node-id placement pinning all behaved as documented; this also surfaced a real gap, now noted in `docs/multi-node.md`, where a control plane started without `APP_AGENT_ADVERTISE_HOST` set lets enrollment succeed but leaves the node stuck at `pending` on a TLS hostname mismatch. This was not run across a real WAN or a second physical host, and the WireGuard mesh itself is unchanged: `internal/network/device_test.go:3-11` still runs against fakes because real encryption "needs two real hosts and root", and the mesh's remote arm (`internal/network.ConfigSink`) does not span nodes yet.
- Label: **beta**. Single node is the stable promise. Multi-node enrollment now has a real, documented verification behind it, but the mesh and cross-host remote transport remain unverified, so the area as a whole stays beta.

### Platform as code (iac)

- Unit: `internal/iac/` (4 files: `fake_test.go`, `filter_test.go`, `iac_test.go`, `schema_file_test.go`) against 17 source files; API `iac_test.go`, `platform_import_test.go`; CLI `apply_test.go`, `apply_vars_test.go`, `import_platform_test.go`; MCP `tools_iac_test.go`; web `IacPlanView.test.tsx`, `PlatformImportCard.test.tsx`.
- E2E: none. Live Docker: none.
- Docs: `docs/platform-as-code.md` (419 lines), `docs/app-spec-reference.md`, `docs/schemas/`.
- Real infra: none found.
- Label: **hide-behind-flag**. The apply path can change many resources in one run and is the least tested relative to its size.

### Log archive

- Unit: `internal/objectstore/archive_test.go`, `internal/alerting/log_archive_stale_test.go`, `internal/mcptools/tools_log_archive_test.go`, `internal/api/storage_destinations_test.go`, CLI `storage_test.go`.
- E2E: `test/e2e/log_archive_test.go` (`TestLogArchive_Live_ContainerLogsToHTTPDownload`): a real container's real stdout through a real `telemetry.LogCollector`, a real `*api.Router` (real admin login, real storage destination created over HTTP), a real manual dump trigger (`POST /api/v1/log-archive/dump`), polled to completion, then listed and downloaded over real HTTP. The bucket itself is `objectstoretest`'s fake S3 server, the same substitution the unit test makes.
- Docs: dedicated page `docs/log-archive.md`, linked from `docs/object-storage.md` and `docs/observability.md`, plus route rows in `docs/api-reference.md`.
- Real infra: none found for the bucket leg. `internal/backup/uploader_live_test.go` (real S3 round trip) is env-gated by `LEVELRAIL_LIVE_S3_*` and its own comment says none of those are set in CI, so it skips there.
- Label: **beta**.

### PITR and database backups

- Unit: `internal/backup/` (34 files, including `pitr_runner_test.go`, `pitr_restore_test.go`, `verify_runner_test.go`, `scheduler_test.go`); `internal/cpbackup/` (9 files) for control-plane backups; CLI `backups_*_test.go`, `control_plane_backups_test.go`, `app_volume_backups_*_test.go`.
- Live Docker: `pitr_live_test.go`, `dump_live_test.go`, `restore_live_test.go`, `clone_restore_live_test.go`, `volume_live_test.go`, `volume_clone_restore_live_test.go`.
- E2E: `test/e2e/pitr_test.go` (real API, real reconciler, MinIO bucket, marker-row restore), `test/e2e/reconcile/database_test.go` (Redis reconcile only, `TestDatabase_Live_RedisReconcile`).
- Docs: `docs/backups-and-storage.md`, `docs/managing-databases.md`, `docs/control-plane-backup.md`, `docs/disaster-recovery.md`. `docs/roadmap.md` notes PITR is "Live-Docker-verified end to end".
- Real infra: none found. The real S3 and R2 test is skipped in CI (see log archive).
- Label: **stable** for PITR and dump/restore/verify on Postgres. Other engines (eight are defined in `internal/store/database.go:16-23`) only have the Redis e2e, so treat them as beta until each has a live restore run.

### Cloudflare tunnel

- Unit: `internal/reconcile/cloudflaretunnel/controller_test.go` and `controller_half_success_test.go`; API `internal/api/cloudflare_tunnel_test.go`; CLI `cloudflare_tunnel_test.go`; MCP `tools_cloudflare.go` covered by `tools_platform_visibility_test.go`.
- E2E: none. Live Docker: none.
- Docs: only in `docs/api-reference.md`, `docs/cli-reference.md`, `docs/feature-catalog.md`. No dedicated page.
- Real infra: none found. Needs a real Cloudflare account and token to prove anything.
- Label: **hide-behind-flag**.

### Deploy approvals

- Unit: `internal/store/deploy_approval_test.go`, `internal/api/deploy_approvals_test.go`, `deploy_approval_safety_test.go`, `deploys_test.go`; CLI `apps_deploys_control_test.go`.
- E2E: `test/e2e/protected_environment_test.go` (`TestProtectedEnvironment_Live_DeployBlockedThenAllowed`): two cookie sessions, same-actor approval rejected, container untouched until a different user approves.
- Docs: `docs/deploy-safety.md` (188 lines).
- Real infra: none found.
- Label: **stable**.

### Deploy freeze

- Unit: `internal/deploy/freeze_test.go`, `internal/store/deploy_safety_test.go`, `internal/api/deploy_safety_test.go`, `deploy_queue_test.go`, `internal/changes/changes_test.go`; CLI `apps_freeze_test.go`.
- E2E: none (no `freeze` match in `test/e2e`).
- Docs: `docs/deploy-safety.md`.
- Real infra: none found.
- Label: **beta**.

### Previews

- Unit: `internal/preview/` (13 files, for example `manager_test.go`, `retention_test.go`, `sqlstore_test.go`); `internal/webhook/pull_request_test.go`; API `preview_test.go`, `preview_comment_test.go`; CLI `apps_previews_test.go`, `apps_previews_policy_test.go`, `preview_mode_test.go`; web `PreviewPolicyCard.test.tsx`, `PreviewSettingsCard.test.tsx`, `PreviewEnvironmentsCard.test.tsx`.
- E2E: `test/e2e/preview_environments_github_test.go` and `preview_environments_gitlab_bitbucket_test.go` (open, synchronize, close lifecycles through real webhook handlers).
- Docs: `docs/deploy-previews.md` (169 lines).
- Real infra: none found. Payloads are synthetic. Recent hardening (`13a50c33`, caps, fork approval, single comment) landed after these tests were written.
- Label: **stable** for GitHub, **beta** for GitLab and Bitbucket.

### Feature flags

- Unit: `internal/store/feature_flag_test.go`, `internal/api/feature_flags_test.go`, CLI `flags_test.go` and `flagutil_test.go`, MCP `tools_new_features_test.go`.
- E2E: none. The `flag` hits in `test/e2e/metrics_test.go` and `live_api_helpers.go` are unrelated.
- Docs: `docs/feature-flags.md` (109 lines). This is an app-level runtime flag system. It is not a platform-level switch for hiding unfinished features. A platform-level hide mechanism does not exist yet.
- Real infra: none found.
- Label: **beta**.

### Templates

- Unit: `internal/catalog/catalog_test.go` (asserts at least 180 templates, unique IDs) and `catalog_extra_test.go`; API `service_templates_test.go`; CLI `templates_test.go`; web `BrowseTemplatesFields.test.tsx`; `internal/registrycatalog/client_test.go`.
- E2E: `test/e2e/template_fleet_test.go` (`TestTemplateFleet_Live_DeploysAndTearsDownCleanly`) deploys a diverse sample of 16 templates through the real one-click route and a real application controller, then tears them down and checks nothing leaks. The other 295 are not deployed by any test. `test/e2e/compose_healthcheck_test.go` covers Compose readiness generally.
- Docs: `docs/templates-and-registry.md` (209 lines).
- Real infra: none found. Only the 16 sampled templates have deploy evidence, so "works" is unverified for the rest. `docs/templates-and-registry.md` also notes GPU passthrough is not translated for GPU templates.
- Count: 311 `ID:` entries across `internal/catalog/templates_*.go`. The roadmap and ADR 015 cite a 339-template goal, which is not yet the shipped catalog.
- Label: **beta**.

### IAM

- Unit: `internal/store/iam_policy_test.go`, `internal/api/iam_handlers_test.go`, `iam_test.go`, `authz_matrix_test.go`, `require_ability_for_resource_test.go`, `app_routes_iam_test.go`, `pipelines_iam_test.go`; CLI `iam_policies_test.go`.
- E2E: `test/e2e/auth_lifecycle_test.go` (`TestAuthLifecycle_Live`) covers sessions, not resource-scoped policy denial.
- Docs: `docs/identity-and-access.md` (529 lines), `docs/security.md`, `docs/threat-model.md`.
- Real infra: none found.
- Label: **beta**. This is the strongest beta candidate. It moves to stable once one e2e shows a denied policy blocking a real action.

### Supply chain

- Unit: `internal/supplychain/` (`gate_fix_test.go`, `parse_test.go`, `service_test.go`); API `supplychain_test.go`; CLI `apps_supply_chain_test.go`; web `SupplyChainSection.test.tsx`, `SupplyChainSettingsCard.test.tsx`.
- E2E: none. Live Docker: none.
- Docs: `docs/supply-chain.md` (94 lines). SBOM and scan are off by default.
- Real infra: none found.
- Label: **beta**, and off by default already.

### Status page

- Unit: `internal/statuspage/statuspage_test.go`; API `status_page_test.go`.
- E2E: none.
- Docs: `docs/status-page.md` (95 lines). Off until enabled.
- Real infra: none found.
- Label: **beta**.

## Numbers quoted across the docs

These counts appear in the README, the landing page, and several guides. Re-check them against the code before changing any of those pages.

| Number | Value | Source |
| --- | --- | --- |
| MCP tools | 156 in full mode, 146 in the default standard mode, 118 in read-only mode | `go test ./internal/mcptools -run TestToolListTokenBudget -v`, and the generated [MCP tool surface](mcp-tool-surface.md) |
| Notification channel kinds | 18 | `NotifyKind` constants in `internal/alerting/rules.go` |
| Alert rule kinds | 14 | `Kind` constants in `internal/alerting/rules.go` |
| Managed database engines | 8 | `internal/store/database.go` |
| Service templates | 311 | `len(catalog.Templates)`, listed in [Template catalog](template-catalog.md) |
| Latest release | 0.2.0-beta.15, so there is no stable release yet | `.release-please-manifest.json` |

Two caveats that still hold. Idle footprint is measured only on an Apple M4 Max dev build (58 to 96 MB RSS from 0 to 500 apps, see [Performance](performance.md)), with no Linux release-build number yet. Real public ACME issuance is built but has no recorded run of [the verification runbook](acme-verification-runbook.md).
