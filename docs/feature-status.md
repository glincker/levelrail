---
title: "Feature status"
description: Per-feature maturity labels (stable, beta, hide-behind-flag) with the test, e2e, docs and real-infrastructure evidence behind each one.
"og:url": "https://docs.levelrail.com/feature-status"
---

This page labels every major feature by how much evidence backs it today. The evidence was first gathered on 2026-09-26. E2E rows were re-checked against main on 2026-10-05. Counts come from the generated pages linked below. Every claim cites a path, and where evidence is missing the page says so.

## How to read the labels

| Label | Meaning |
| --- | --- |
| **stable** | Unit tests, a doc page, and either a live end-to-end test in `test/e2e` or a documented live verification. |
| **beta** | Works and is tested, but has no end-to-end test and no record of a run on real infrastructure. Expect rough edges. |
| **hide-behind-flag** | Hardware-dependent or outside the core single-node story, or too thinly evidenced. Off by default and enabled with `APP_EXPERIMENTAL` (see [Experimental features](/experimental-features)). |

Evidence tiers used below, weakest to strongest:

1. Unit: `*_test.go` against fakes or `httptest` servers.
2. Live Docker: tests named `*_Live_*` or `*_live_test.go` that skip when no Docker daemon is reachable and run against a real local daemon (for example `internal/pipeline/live_test.go`, `internal/backup/pitr_live_test.go`). `nightly.yml` has a `docker` lane, but `test/e2e` itself is only named in the flake sweep (`.github/workflows/nightly.yml:111`).
3. E2E: `test/e2e/*_test.go`, which drive the real HTTP API and a real reconciler against real containers on one machine.
4. Real infrastructure: a fresh VPS, a real public domain, real vendor endpoints. Almost no feature has this evidence in the repository. [Multi-node](/multi-node) enrollment is the one documented exception so far, verified locally across two real Docker daemons (see its own section below), not against a real VPS or a real WAN. Real public ACME now has one recorded run against a live public VPS (`docs/acme-verification-runbook.md`, 2026-10-05); the e2e suite itself still "does not yet exercise a full multi-node mesh or real ACME against a live domain".

Because tier 4 is empty for almost every row, the "real infra" column below reads "none found" outside multi-node. What matters is the gap between tier 3 and tiers 1 and 2.

## Summary

| Area | Label | One-line justification |
| --- | --- | --- |
| Deploy approvals | stable | Live e2e proves a second user's approval is the only thing that cuts over (`test/e2e/protected_environment_test.go`). |
| Upgrade restore points and post-upgrade verification | beta | Unit tests (`internal/selfupgrade/restorepoint_test.go`) plus a harness that upgrades the real v0.8.1 and v0.8.2 binaries with seeded data, then checks the restore point, an automatic restore after a failed verification and a refused downgrade (`scripts/upgrade-harness.sh`, `TestReleaseUpgradeHarness`). Not yet proven: a migration that changes the schema across releases, systemd and agent reconnection, or a live host. See [Upgrade safely](/upgrade-safely). |
| PITR and database backups (Postgres) | stable | Live e2e restores before a marker row through the real API with an S3 compatible bucket (`test/e2e/pitr_test.go`), plus live Docker dump and restore tests. |
| MySQL and MariaDB PITR, TLS on new databases | beta | Live e2e through the real API, reconciler and bucket: binlog replay into a new database keeps a row written after the base backup and drops one written after the target (`test/e2e/pitr_mysql_test.go`), and new instances serve the platform certificate while older ones are untouched. One local Docker run, not yet in the nightly lane; DDL inside the replay window is untested. |
| MongoDB backup, verify, restore and TLS on new databases | beta | Live e2e: dump, verify and restore-as-new through the API (`test/e2e/engine_parity_test.go`), and a new instance accepts TLS while an older one does not. No point-in-time restore: standalone mongod has no oplog. |
| Encrypted database dumps | beta | Per-target age sealing of dumps with transparent restore, drill and verify decryption; unit round trip only (`internal/backup/encrypted_dump_test.go`), no live Docker run. |
| Previews | stable (GitHub), beta (GitLab, Bitbucket) | Lifecycle e2e exists for all three (`test/e2e/preview_environments_*_test.go`), but they replay synthetic webhooks, not a real forge. Kept beta for the two forges with less usage. |
| IAM | stable | Broadest unit coverage in the repo, plus live e2e where a deny policy blocks a real action and a token allow policy scopes one (`test/e2e/iam_policy_enforcement_test.go`). |
| SAML single sign-on and SCIM provisioning | beta | Built on the sign-in library, behind the `saml-scim` feature toggle, off by default. SAML was run against Keycloak 26.0.8 and SimpleSAMLphp 1.15 and generic OIDC against Keycloak 26.0.8 in a local lab (2026-10-20, `scripts/sso-lab.sh`). Not tested against Okta or Entra ID. See [Identity and access](#identity-and-access). |
| Notification channels: generic webhook and email over SMTP | stable | Live e2e against a local receiver and a local SMTP server: delivery, retry of a transient failure and delivery history (`test/e2e/notification_channels_test.go`). Both are open protocols with no vendor account, so a local fake exercises the real contract. |
| Notification channels: 15 vendor kinds and browser push | beta | Slack, Discord, Telegram, Pushover, PagerDuty, Teams, Resend, ntfy, Gotify, Mattermost, Lark, Rocket.Chat, Opsgenie, Webex and Google Chat now have e2e against local fakes that check each vendor's documented payload shape and auth header. No real vendor has accepted a send, so they stay beta. Browser push is tested with a recording sender only. |
| Pipelines | stable | Live Docker test (`internal/pipeline/live_test.go`) and a live e2e run (`test/e2e/pipeline_run_test.go`). |
| Deploy freeze | stable | Live e2e proves a freeze blocks then allows a real deploy (`test/e2e/deploy_freeze_test.go`), documented in `docs/deploy-safety.md`. |
| Post-deploy watch and auto rollback | stable (hard signals), beta (latency signal) | Live e2e on a real Docker engine and real ingress: a 5xx spike and a post-cutover crashloop each roll back to the previous pinned image, recorded in deploy history, the gate timeline, the audit log and a notification, and a healthy release is left alone (`test/e2e/reconcile/deploy_safety_live_test.go`, `deploy_watch_rollback_live_test.go`). The soft p95 latency signal is unit tested only. |
| Feature flags | stable | Live e2e drives create, evaluate with a read-only token, kill switch, rollout, per-app scoping and delete through the API, the real CLI binary and the MCP read tools (`test/e2e/feature_flags_test.go`). Per-app only: no per-environment or attribute targeting (`docs/feature-flags.md`). |
| Templates | beta | About 360 catalog entries (generated count in the [catalog](/template-catalog)). Unit tests check shape and a floor of 180, and live e2e tests deploy samples (a 16-template fleet plus batch boot tests), not the whole catalog. |
| Log archive | beta | Unit and e2e tests (`test/e2e/log_archive_test.go`), dedicated doc page (`docs/log-archive.md`). |
| Automatic cleanup (janitor) | beta | Unit tests for every stage and safety rule, API test, CLI, MCP read tool, doc page (`docs/janitor.md`). No live disk-fill run yet; other nodes are not covered. |
| Resilience check (data plane without the control plane) | beta | Proven in tests: route table keeps answering through a stalled control plane, sealed snapshot restores with the database down, the dry run flags stale or corrupt snapshots. Not proven: a real process kill under load on a real host. Known gap, listed not fixed: containers use restart policy `no`, so a host reboot during an outage leaves apps down; agents do not buffer telemetry (it stays node local, size not reported). |
| Node capacity guardrails | beta | Unit tests for thresholds and for preview-then-confirm cleanup against a fake Docker client (rollback images and volumes untouched). Cleanup covers the control plane node only; remote nodes monitor and block. No live disk-fill run yet. |
| Supply chain | stable | Live e2e proves the gate blocks and an override works through the real API (`test/e2e/supply_chain_test.go`). SBOM and scan stay off by default (`docs/supply-chain.md`). |
| Sleep when idle | stable | Unit tests across scheduler, API, CLI and web, and a documented live run against a real Docker daemon and the real ingress (`scripts/verify-sleep-live.sh`, 10 checks: idle stop, wake on request, hold mode, manual wake, fleet counts). Per app and off by default (`docs/sleep-when-idle.md`). |
| Status page | stable | Live e2e enables, authors and serves it publicly (`test/e2e/status_page_test.go`). Off until enabled. |
| Multi-node and WireGuard | beta | Opt-in node failure policy (fakes only, no real hosts). Join flow verified locally across two real Docker daemons (enrollment, cordon, drain with real container relocation); the WireGuard mesh itself and cross-host remote transport are still unverified. |
| AI assistant (MCP server) | beta | over 240 tools in toolsets an agent enables on demand (exact, generated numbers on the [MCP tool surface](/mcp-tool-surface) page), 31 test files in `internal/mcptools`, no e2e. |
| AI assistant (in-app chat) | hide-behind-flag | Tested against fake Anthropic responses only (unit, plus a real-HTTP e2e session lifecycle test with a fake provider); `docs/ai-assistant-chat.md` now documents it. |
| AI models (GPU) | hide-behind-flag | Needs NVIDIA hardware, every test uses fakes, docs state v1 scope is NVIDIA on Linux only. |
| Load balancer | hide-behind-flag | Live e2e with real traffic and drain (`test/e2e/loadbalancer_test.go`), but gated behind `load-balancer` and not exercised across real hosts. |
| Platform as code (iac) | hide-behind-flag | Live e2e covers export, plan and apply (`test/e2e/iac_apply_test.go`), but the apply path changes many resources in one run and stays gated behind `iac`. |
| Cloudflare tunnel | hide-behind-flag | Reconciler unit tests only, needs a real Cloudflare account, no dedicated doc page. |

## Identity and access

What exists for sign-in, authorization and audit, with the label each one carries. Only IAM and the SAML and SCIM toggle have their own label in this page; the rest is core sign-in that ships on by default and has unit tests, with `test/e2e/auth_lifecycle_test.go` covering sessions.

| Capability | Maturity | Notes |
| --- | --- | --- |
| Password sign-in | core, on by default | Local accounts with sessions. |
| Passkeys (WebAuthn) | core, on by default | Register and sign in, with a guard for passkey-only rules (`internal/api/passkey_guard_test.go`). |
| TOTP two-factor | core, on by default | Recovery codes included (`internal/authengine/mfa_test.go`). |
| OAuth and OIDC sign-in | core, on by default once a provider is configured | 17 named providers plus generic OpenID Connect, 18 settings rows in all (`internal/store/oauth_identity.go`). Unit tests use a fake identity provider; the generic OIDC provider was also run against Keycloak 26.0.8 (code flow with PKCE, account creation, group to role mapping, domain refusal, no root from SSO). Not tested against Okta, Entra ID, Google or Cognito. [OIDC](/sso-oidc) |
| SAML single sign-on | beta | Built on the sign-in library, behind the `saml-scim` feature toggle (Settings, Beta features), off by default. Verified against Keycloak 26.0.8 and SimpleSAMLphp 1.15 in a local lab on 2026-10-20; not tested against Okta or Entra ID. [SAML](/sso-saml) |
| SCIM provisioning | beta | Same toggle and same caveats. Users and groups under `/api/v1/scim/v2`. [SCIM](/scim) |
| IAM policies | stable | See the IAM label above. |
| Teams and roles | on by default | Owner, admin, developer and viewer roles per team. [Teams and roles](/teams-and-roles). Custom roles and per-environment grants are experimental (`access-roles`). |
| Audit log | on by default | Every mutating request recorded, hash chain you can verify (`GET /api/v1/audit-log/verify`), CSV export, retention, and forwarding through [Data export](/data-export). |

LDAP directory sync is not supported.

## Evidence per area

Test counts are `*_test.go` files in the named directory. "CLI" and "web" list test files found in `cmd/levelrail-cli` and `web/src`.

### AI assistant (MCP server and in-app chat)

- Unit: `internal/ai/` (5 files: `anthropic_test.go`, `classify_test.go`, `engine_test.go`, `eval_injection_test.go`, `toolcaller_test.go`); `internal/mcptools/` (31 files, for example `modes_test.go`, `tools_apps_clone_images_test.go`); API `internal/api/ai_chat_test.go`, `internal/api/ai_settings_test.go`; CLI `cmd/levelrail-cli/settings_ai_assistant_test.go`; web `web/src/components/AiChatMessageList.test.tsx`, `AiToolCallCard.test.tsx`, `web/src/queries/aiAssistant.test.ts`, `web/src/components/AiChatHistoryMenu.test.tsx`.
- E2E: `test/e2e/ai_chat_test.go` drives create/send-message/get/list/delete over real HTTP against a real `*api.Router` and `ai.Engine`, with a fake `ai.Provider` and `ai.ToolExecutor` (no live Anthropic call). MCP server itself: none.
- Docs: `docs/ai-assistant.md` covers `levelrail-mcp`; `docs/ai-assistant-chat.md` covers the in-app chat.
- Real infra: none found.
- Label: MCP server beta (the tool registry is well tested, but the surface is large and unexercised end to end). In-app chat hide-behind-flag (depends on an external model provider, no evidence beyond fakes).

### AI models (GPU inference)

- Unit: `internal/models/` (13 files, for example `gpunodes_test.go`, `engines_test.go`, `gateway_test.go`, `prober_test.go`); API `internal/api/models_test.go`, `models_hf_test.go`, `models_keys_test.go`; CLI `models_test.go`, `models_keys_test.go`, `models_preflight_test.go`; web `ModelRow.test.tsx`, `ModelCacheCard.test.tsx`, `ModelKeysPanel.test.tsx`, `ModelPreflightPanel.test.tsx`.
- E2E: none. No live Docker test either.
- Docs: `docs/ai-models.md`. It states v1 supports NVIDIA GPUs on Linux only.
- Real infra: none found. No test touches a real GPU.
- Label: hide-behind-flag. Correctness depends on driver, container toolkit and VRAM behavior that no test in the repo observes.

### Load balancer

- Unit: `internal/loadbalancer/` (4 files: `check_test.go`, `config_test.go`, `history_test.go`, `status_test.go`); API `loadbalancer_test.go`, `loadbalancer_history_test.go`, `loadbalancer_list_test.go`; CLI `lb_test.go`, `lb_history_test.go`, `lb_list_test.go`; web `LoadBalancerOverview.test.tsx`, `web/src/lib/loadBalancer.test.ts`; MCP `tools_loadbalancer_test.go` and two more.
- E2E: `test/e2e/loadbalancer_test.go` (`TestLoadBalancer_Live_RealTrafficAndDrain`, plus a gate-off test that expects 404). Live Docker: none beyond that.
- Docs: `docs/load-balancing.md`.
- Real infra: none found.
- Label: hide-behind-flag. Multi-replica routing is built on embedded Caddy and verified on one machine only.

### Pipelines

- Unit: `internal/pipeline/` (14 files, including `engine_test.go`, `definition_test.go`, `trigger_event_test.go`, `pathtrigger_test.go`, `sync_test.go`); API `pipelines_test.go`, `pipelines_events_ci_test.go`, `pipelines_iam_test.go`, `pipelines_sync_test.go`; CLI `pipelines_test.go`, `pipelines_all_test.go`, `pipelines_sync_test.go`; web has six `Pipeline*.test.tsx` files.
- Live Docker: `internal/pipeline/live_test.go` runs a two-job pipeline with artifact handoff and a failing step, then checks nothing leaks.
- E2E: `test/e2e/pipeline_run_test.go` (`TestPipelineRun_Live`).
- Docs: `docs/pipelines.md`.
- Real infra: none found.
- Label: stable.

### Email delivery

Seven providers behind one registry (`internal/email`): SMTP, AWS SES over the API, AWS SES over SMTP, Resend, Postmark, SendGrid and Mailgun. Settings page groups Sender, Provider, Delivery limits and Security, with a status header, a typed-failure test send, a SPF/DKIM/DMARC sender-domain check, and a searchable delivery log (`/settings/email-log`, retention `APP_EMAIL_LOG_RETENTION_DAYS`). Transient failures retry with backoff. Alert kind `email_delivery_failing` is registered with no default rule.

- Unit: `internal/email/regions_test.go` (region list, SES SMTP host, provider registry, failure classification, retry, caps), `internal/store/email_deliveries_test.go` (retention, recipient privacy), `cmd/levelrail/boot_email_provider_test.go`; web `EmailSettingsSchema.test.ts`.
- CLI: `email settings get|set`, `email test`, `email check-domain`, `email log`. Docs: `docs/email-notifications.md`.
- Real infra: none. Provider calls are not exercised against live accounts.
- Label: beta.

### Notification channels (all 18)

Kinds, from the `NotifyKind` constants in `internal/alerting/rules.go`: generic, slack, discord, telegram, email, pushover, pagerduty, teams, resend, ntfy, gotify, mattermost, lark, rocketchat, opsgenie, webex, googlechat, webpush. That is 18, matching the README and comparison count. `webpush` (browser push, `internal/webpush/`) is the newest and the only kind whose destination isn't an operator-supplied URL: it fans out to every registered browser subscription instead.

- Unit: `internal/alerting/` (28 files). Payload tests live in `notify_test.go` (`TestNotifyTelegram_PostsChatIDAndText`, `TestNotifyResend_PostsAuthHeaderAndPayload`, `TestNotifyOpsgenie_PostsAuthHeaderAndPayload`) and `TestNewNotifier_AllValidKinds_Recognized` (`notify_test.go:1095`). That table test covers a subset of the kinds, so the rest rely on their own payload tests, and coverage is thinnest for pushover, pagerduty and teams.
- API `notification_channels_test.go`; CLI: no dedicated channel test file found; email retry tests at `notify_test.go:569-629`.
- E2E: `test/e2e/notification_channels_test.go` with a local fake-vendor harness (`notification_harness_test.go`: httptest receivers, a host redirect for the fixed PagerDuty, Opsgenie and Resend endpoints, a minimal SMTP server). Through the real router it creates a channel, sends a test, checks the vendor payload and auth header, injects one 503 (or SMTP 421) and expects a retry, and for the generic webhook, Slack, Discord, PagerDuty and SMTP also injects a hard outage and expects exactly 3 attempts and a failed delivery row. A second test sends a failed-deploy outcome through the deploy dispatcher to Slack (with a transient 503) and PagerDuty, and checks the `deploy-failed` history rows. Not covered: the alert-rule engine path (rule fires, then notifies) is still unit-only.
- Docs: covered in `docs/integrations.md` and `docs/observability.md`.
- Real infra: none found. Every vendor kind was tested against a local fake, not the real vendor. A fake proves the request matches the vendor's documented contract, not that a real account accepts it (rate limits, regional endpoints such as Opsgenie EU, signing and account setup are untested). The e2e found one real mismatch: ntfy JSON was posted to the topic path, where ntfy publishes it as literal text; it now posts to the server root with the topic in the body.
- Label: generic webhook and SMTP email stable. The 15 vendor kinds and browser push stay beta: run a manual send test per vendor before calling any of them stable.

### Multi-node and WireGuard mesh

- Unit: `internal/network/` (13 files), `internal/agent/` (about 20 files including `grpc_transport_test.go`, `pki_test.go`, `renew_test.go`, `mesh_dispatch_test.go`), `cmd/levelrail/mesh_test.go` (skips without Docker); CLI `nodes_*_test.go` (about 14 files); reconcile `internal/reconcile/mesh/controller_test.go`.
- Live Docker: `internal/agent/live_test.go`, `internal/agent/build_live_test.go`, `internal/reconcile/application/placement_live_test.go`.
- E2E: `test/e2e/node_placement_test.go`. Its own header says one Docker daemon cannot exercise the NodeID-to-remote-agent resolution and that it only proves each controller uses the runtime it was built with.
- Docs: `docs/multi-node.md`.
- Real infra: the join flow was verified against two real agent processes, each with its own real Docker daemon (separate `docker:27-dind` containers), running against a real control plane binary. Enrollment, cordon, drain with actual container relocation between the two daemons, and explicit node-id placement pinning all behaved as documented; this also surfaced a real gap, now noted in `docs/multi-node.md`, where a control plane started without `APP_AGENT_ADVERTISE_HOST` set lets enrollment succeed but leaves the node stuck at `pending` on a TLS hostname mismatch. This was not run across a real WAN or a second physical host, and the WireGuard mesh itself is unchanged: `internal/network/device_test.go:3-11` still runs against fakes because real encryption "needs two real hosts and root", and the mesh's remote arm (`internal/network.ConfigSink`) does not span nodes yet.
- Label: beta. Single node is the stable promise. Multi-node enrollment now has a documented real verification behind it, but the mesh and cross-host remote transport are still unverified, so the area stays beta.

### Platform as code (iac)

- Unit: `internal/iac/` (4 files: `fake_test.go`, `filter_test.go`, `iac_test.go`, `schema_file_test.go`) against 17 source files; API `iac_test.go`, `platform_import_test.go`; CLI `apply_test.go`, `apply_vars_test.go`, `import_platform_test.go`; MCP `tools_iac_test.go`; web `IacPlanView.test.tsx`, `PlatformImportCard.test.tsx`.
- E2E: `test/e2e/iac_apply_test.go` (`TestIaCApply_Live_ExportPlanApplyRoundTrip`, plus a gate-off test that expects 404). Live Docker: none beyond that.
- Docs: `docs/platform-as-code.md`, `docs/app-spec-reference.md`, `docs/schemas/`.
- Real infra: none found.
- Label: hide-behind-flag. The apply path can change many resources in one run, so it stays opt-in until it has more real-world runs.

### Log archive

- Unit: `internal/objectstore/archive_test.go`, `internal/alerting/log_archive_stale_test.go`, `internal/mcptools/tools_log_archive_test.go`, `internal/api/storage_destinations_test.go`, CLI `storage_test.go`.
- E2E: `test/e2e/log_archive_test.go` (`TestLogArchive_Live_ContainerLogsToHTTPDownload`): a real container's real stdout through a real `telemetry.LogCollector`, a real `*api.Router` (real admin login, real storage destination created over HTTP), a real manual dump trigger (`POST /api/v1/log-archive/dump`), polled to completion, then listed and downloaded over real HTTP. The bucket itself is `objectstoretest`'s fake S3 server, the same substitution the unit test makes.
- Docs: dedicated page `docs/log-archive.md`, linked from `docs/object-storage.md` and `docs/observability.md`, plus route rows in `docs/api-reference.md`.
- Real infra: none found for the bucket leg. `internal/backup/uploader_live_test.go` (real S3 round trip) is env-gated by `LEVELRAIL_LIVE_S3_*` and its own comment says none of those are set in CI, so it skips there.
- Label: beta.

### PITR and database backups

- Unit: `internal/backup/` (34 files, including `pitr_runner_test.go`, `pitr_restore_test.go`, `verify_runner_test.go`, `scheduler_test.go`); `internal/cpbackup/` (9 files) for control-plane backups; CLI `backups_*_test.go`, `control_plane_backups_test.go`, `app_volume_backups_*_test.go`.
- Live Docker: `pitr_live_test.go`, `dump_live_test.go`, `restore_live_test.go`, `clone_restore_live_test.go`, `volume_live_test.go`, `volume_clone_restore_live_test.go`.
- E2E: `test/e2e/pitr_test.go` (real API, real reconciler, S3 compatible bucket, marker-row restore), `test/e2e/pitr_mysql_test.go` (MySQL and MariaDB PITR and TLS), `test/e2e/engine_parity_test.go` (MySQL, MariaDB and MongoDB backup, verify, restore-as-new, MongoDB TLS), `test/e2e/reconcile/database_test.go` (Redis reconcile only, `TestDatabase_Live_RedisReconcile`). The e2e bucket is SeaweedFS because MinIO's free images were retired.
- Docs: `docs/backups-and-storage.md`, `docs/managing-databases.md`, `docs/control-plane-backup.md`, `docs/disaster-recovery.md`. `docs/roadmap.md` notes PITR is "Live-Docker-verified end to end".
- Real infra: none found. The real S3 and R2 test is skipped in CI (see [log archive](/log-archive)).
- Label: stable for PITR and dump/restore/verify on Postgres. MySQL, MariaDB and MongoDB are beta: each now has a live API e2e for dump, verify and restore, and MySQL and MariaDB for PITR, but only from a local run. The remaining engines (13 are defined as constants in `internal/store/database.go`) only have the Redis reconcile e2e, so treat them as beta until each has a live restore run.

### Cloudflare tunnel

- Unit: `internal/reconcile/cloudflaretunnel/controller_test.go` and `controller_half_success_test.go`; API `internal/api/cloudflare_tunnel_test.go`; CLI `cloudflare_tunnel_test.go`; MCP `tools_cloudflare.go` covered by `tools_platform_visibility_test.go`.
- E2E: none. Live Docker: none.
- Docs: only in `docs/api-reference.md`, `docs/cli-reference.md`, `docs/feature-catalog.md`. No dedicated page.
- Real infra: none found. Needs a real Cloudflare account and token to prove anything.
- Label: hide-behind-flag.

### Deploy approvals

- Unit: `internal/store/deploy_approval_test.go`, `internal/api/deploy_approvals_test.go`, `deploy_approval_safety_test.go`, `deploys_test.go`; CLI `apps_deploys_control_test.go`.
- E2E: `test/e2e/protected_environment_test.go` (`TestProtectedEnvironment_Live_DeployBlockedThenAllowed`): two cookie sessions, same-actor approval rejected, container untouched until a different user approves.
- Docs: `docs/deploy-safety.md`.
- Real infra: none found.
- Label: stable.

### Post-deploy watch and auto rollback

- Unit: `internal/deploywatch/watcher_test.go`, `watcher_soft_test.go`, `internal/deploysafety/rules_test.go`; CLI `apps_deploys_gate_test.go`.
- E2E: `test/e2e/reconcile/deploy_safety_live_test.go` (zero failed requests through a good deploy, a crash inside the stable window and a 5xx rollback) and `deploy_watch_rollback_live_test.go` (a healthy release passes its whole watch window with no rollback and no audit entry; a release that crashes after cutover trips `PostDeployCrashloop`, is rolled back to the previous image, and shows up as an `auto_rollback` deploy attempt, a rollback gate event, a `deploy.auto_rollback` audit entry and an alert). These tests had been silently running with automatic rollback off because the harness left the watch mode unset; they now set it, and they run again. The harness uses 3 second windows, not production defaults.
- Docs: `docs/deploy-safety.md`.
- Real infra: none found. The latency signal has no e2e, and nothing was observed under real production traffic.
- Label: stable for the hard signals (crashloop, OOM, 5xx). beta for the soft latency signal.

### Deploy freeze

- Unit: `internal/deploy/freeze_test.go`, `internal/store/deploy_safety_test.go`, `internal/api/deploy_safety_test.go`, `deploy_queue_test.go`, `internal/changes/changes_test.go`; CLI `apps_freeze_test.go`.
- E2E: `test/e2e/deploy_freeze_test.go` (`TestDeployFreeze_Live_BlocksThenAllows`).
- Docs: `docs/deploy-safety.md`.
- Real infra: none found.
- Label: stable.

### Previews

- Unit: `internal/preview/` (13 files, for example `manager_test.go`, `retention_test.go`, `sqlstore_test.go`); `internal/webhook/pull_request_test.go`; API `preview_test.go`, `preview_comment_test.go`; CLI `apps_previews_test.go`, `apps_previews_policy_test.go`, `preview_mode_test.go`; web `PreviewPolicyCard.test.tsx`, `PreviewSettingsCard.test.tsx`, `PreviewEnvironmentsCard.test.tsx`.
- E2E: `test/e2e/preview_environments_github_test.go` and `preview_environments_gitlab_bitbucket_test.go` (open, synchronize, close lifecycles through real webhook handlers).
- Docs: `docs/deploy-previews.md`.
- Real infra: none found. Payloads are synthetic. Recent hardening (`13a50c33`, caps, fork approval, single comment) landed after these tests were written.
- Label: stable for GitHub, beta for GitLab and Bitbucket.

### Feature flags

- Unit: `internal/store/feature_flag_test.go`, `internal/api/feature_flags_test.go`, CLI `flags_test.go` and `flagutil_test.go`, MCP `tools_new_features_test.go`.
- E2E: `test/e2e/feature_flags_test.go` (`TestFeatureFlags_Live_APICLIAndMCP`): a real router and store, the real `levelrail-cli flags` binary and the real MCP read tools. It checks create, evaluate with a read-only token, duplicate key across apps, kill switch, 0 and 50 percent rollout (stable per identifier), a read-only token refused on write, per-app listing, delete through the CLI and the API, and the audit log entries. MCP exposes read tools only, by design; evaluate is API only. There is no per-environment targeting in the product, so none is tested.
- Docs: `docs/feature-flags.md`. This is an app-level runtime flag system. It is not a platform-level switch for hiding unfinished features. A platform-level hide mechanism does not exist yet.
- Real infra: none found.
- Label: stable.

### Templates

- Unit: `internal/catalog/catalog_test.go` (asserts at least 180 templates, unique IDs) and `catalog_extra_test.go`; API `service_templates_test.go`; CLI `templates_test.go`; web `BrowseTemplatesFields.test.tsx`; `internal/registrycatalog/client_test.go`.
- E2E: `test/e2e/template_fleet_test.go` (`TestTemplateFleet_Live_DeploysAndTearsDownCleanly`) deploys a diverse sample of 16 templates through the real one-click route and a real application controller, then tears them down and checks nothing leaks. Batch boot tests (`catalog_batch*_live_test.go`, `catalog_devtools_batch_live_test.go`) cover more, but not the whole catalog. `test/e2e/compose_healthcheck_test.go` covers Compose readiness generally.
- Docs: `docs/templates-and-registry.md`.
- Real infra: none found. Only the 16 sampled templates have deploy evidence, so "works" is unverified for the rest. `docs/templates-and-registry.md` also notes GPU passthrough is not translated for GPU templates.
- Count: about 360 entries in `internal/catalog.Templates` (359 in this build, see [Template catalog](/template-catalog)). ADR 015 cites a 339-template goal, which is not yet the shipped catalog.
- Label: beta.

### IAM

- Unit: `internal/store/iam_policy_test.go`, `internal/api/iam_handlers_test.go`, `iam_test.go`, `authz_matrix_test.go`, `require_ability_for_resource_test.go`, `app_routes_iam_test.go`, `pipelines_iam_test.go`; CLI `iam_policies_test.go`.
- E2E: `test/e2e/auth_lifecycle_test.go` (`TestAuthLifecycle_Live`) covers sessions. `test/e2e/iam_policy_enforcement_test.go` covers a session deny policy blocking a real action and a token allow policy scoping one.
- Docs: `docs/identity-and-access.md`, `docs/security.md`, `docs/threat-model.md`.
- Real infra: none found.
- Label: stable.

### Supply chain

- Unit: `internal/supplychain/` (`gate_fix_test.go`, `parse_test.go`, `service_test.go`); API `supplychain_test.go`; CLI `apps_supply_chain_test.go`; web `SupplyChainSection.test.tsx`, `SupplyChainSettingsCard.test.tsx`.
- E2E: `test/e2e/supply_chain_test.go` (`TestSupplyChain_Live_GateBlocksAndOverrideThroughRealAPI`). Live Docker: none beyond that.
- Docs: `docs/supply-chain.md`. SBOM and scan are off by default.
- Real infra: none found.
- Label: stable, and off by default.

### Sleep when idle

- Unit: `internal/appsleep/appsleep_test.go` (idle, busy, fresh, hand-stopped, hand-started and pinned-port apps), `internal/api/app_sleep_test.go` and `app_sleep_fleet_test.go` (settings, wake hook token, sleep now, pinned-port refusal, fleet counts), `cmd/levelrail-cli/apps_sleep_test.go`, `web/src/components/AppSleepCard.test.tsx`.
- Live: `scripts/verify-sleep-live.sh` builds the control plane, runs it against a real Docker daemon with the embedded ingress, and asserts container state with `docker ps` at every step: request through the ingress, sleep now stops the container, the first request returns the waking page and starts it, hold mode answers with one redirect then 200, wake now restarts it, turning sleep off clears the fleet view. Run on macOS with Docker Desktop (2026-10-14).
- Docs: `docs/sleep-when-idle.md`, including the limits below.
- Real infra: none found. Not run on a Linux VPS or against a real public domain.
- Known limits, all documented: activity is seen only through the built-in ingress (TCP streams, pinned host ports and app-to-app traffic do not count, and an app with a pinned host port is refused); background jobs do not count; the first visitor waits for the cold start.
- Label: stable.

### Status page

- Unit: `internal/statuspage/statuspage_test.go`; API `status_page_test.go`.
- E2E: `test/e2e/status_page_test.go` (`TestStatusPage_Live_EnableAuthorAndServePublicly`).
- Docs: `docs/status-page.md`. Off until enabled.
- Real infra: none found.
- Label: stable.

## Numbers quoted across the docs

These counts appear in the README, the landing page, and several guides. Re-check them against the code before changing any of those pages.

| Number | Value | Source |
| --- | --- | --- |
| MCP tools | over 240, grouped into on-demand toolsets | The generated [MCP tool surface](/mcp-tool-surface) |
| Notification channel kinds | 18 | `NotifyKind` constants in `internal/alerting/rules.go` |
| Alert rule kinds | 25 | `Kind` constants in `internal/alerting` |
| Managed database engines | 9 supported; 4 more built but not yet live-verified | `internal/store/database.go`, [Managing databases](/managing-databases) |
| Service templates | about 360 | `len(catalog.Templates)`, listed in [Template catalog](/template-catalog) |
| Latest release | See the [releases page](https://github.com/glincker/levelrail/releases) | `.github/release-please/.release-please-manifest.json` |

Two caveats still hold. The headline idle footprint is about 200 MB RSS and 1.5 percent CPU on one 2 vCPU, 8 GB Linux test box with no apps under load ([Benchmarks](/benchmarks#control-plane-idle-footprint-2026-10-19)). The 58 to 96 MB figures in [Performance](/performance) come from an Apple M4 Max dev build and are not comparable. The 100 and 500 app numbers are from that dev build only. Real public ACME issuance has one recorded live run ([the verification runbook](/acme-verification-runbook), 2026-10-05); renewal and DNS-01 wildcards are still unproven.
