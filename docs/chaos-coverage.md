---
title: "Chaos test coverage"
description: "Which failure scenarios have a test, what each test asserts, and whether it runs under -short with a fake Docker client or live against a real daemon."
"og:url": "https://docs.levelrail.com/chaos-coverage"
---

Phase 5 promises one test per failure mode. This page maps each scenario to the tests that prove it, so a gap is visible instead of assumed. Every fast test also asserts the level-triggered property: after recovery, one more reconcile pass is a no-op (no create, start, stop or remove).

**Fast** tests use the fake Docker client with injected failures and run under `go test -short`. **Live** tests need a Docker daemon, live in `_live_test.go` files, and skip under `-short`; the nightly run executes them.

## Matrix

| Scenario | What is asserted | Test | Kind | Status |
| --- | --- | --- | --- | --- |
| Docker daemon dies mid-deploy | Old release keeps running, failed pass has a stable False reason, no orphan, recovery converges, next pass is a no-op | `TestChaos_DockerDaemonDiesMidDeploy_NoOrphanAndConverges` (`internal/reconcile/application`) | Fast | Covered |
| Daemon dies after create (start, cleanup and inspect fail) | One create only, old release survives a fully down daemon with zero mutations, a restarted controller converges to one container, then a no-op | `TestChaosPack_DaemonDiesAfterCreate_OldReleaseSurvivesAndConverges` | Fast | Covered |
| Daemon blip at any inspect step, three strategies | Old release never stopped, at most old plus new present, recovery converges, then a no-op | `TestChaosPack_DaemonBlipAtEveryInspectStep_ConvergesWithoutOrphans` | Fast | Covered |
| Daemon unreachable before any change | Reason `InspectFailed`, zero mutations, recovers to `Deployed` | `TestController_Reconcile_DockerUnreachable_ReportsInspectFailedAndChangesNothing` | Fast | Covered |
| Daemon dies mid-deploy, real containers | Real old container keeps running, real recovery removes it exactly once, container count stays 1 on a no-op pass | `TestChaosPack_Live_DaemonDiesMidDeploy_OldKeepsServingAndConverges` | Live | Covered |
| Daemon failure classified | Message maps to `docker_unreachable` with a cause and a fix | `TestChaos_EveryFailureHasAConditionAndFix` (`test/chaos`, `-tags chaos`) | Fast | Covered |
| Disk full at image pull, container start and volume ensure | Failure classifies as `disk_full`, old release keeps serving, no extra running container, freeing space converges, then a no-op | `TestChaosPack_DiskFull_AtEachWriteStep_OldKeepsServingAndIsTyped` | Fast | Covered |
| Disk full at image pull (reason) | Reason `CreateFailed`, old release serving, converges | `TestChaos_DiskFullDuringImagePull_OldKeepsServing` | Fast | Covered |
| Disk full during a build | Classifies as `disk_full`, desired state is not replaced, retry saves exactly once | `TestChaosPack_DiskFullDuringBuild_TypedNoPartialStateAndRetryConverges` (`internal/deploy`) | Fast | Covered |
| Disk full during a deploy, real containers | Real old container keeps running, no partial release, converges after space returns, then a no-op | `TestChaosPack_Live_DiskFullDuringDeploy_OldKeepsServingAndConverges` | Live | Covered |
| Disk full on a metric write | Release stays Ready (`DeployedMetricRecordFailed`), containers unchanged, the next pass does not redeploy or record again | `TestChaosPack_DiskFullDuringMetricWrite_DeployStaysReadyAndIsNotRepeated` | Fast | Covered |
| Disk full on a build log write | Live viewers keep getting lines, nothing partial stored, next attempt persists | `TestChaosPack_DiskFullOnLogWrite_LiveFeedIntactAndNextAttemptPersists` (`internal/deploylog`) | Fast | Covered |
| Disk guard refuses before the disk fills | Refuses build and backup at the floor, releases then restores its reserve | `TestChaos_DiskFillRefusesThenRecovers` (`test/chaos`) | Fast | Covered |
| SQLite hits ENOSPC mid transaction | Rollback leaves the database intact and writable | `TestChaos_SQLiteSurvivesNoSpaceMidTransaction` (`test/chaos`) | Fast | Covered |
| Agent connection severed before a pass | Every pass fails without mutating, old release keeps running, reconnect converges to one container, then a no-op | `TestChaosPack_AgentSeveredBeforePass_NothingMutatedAndReapplyIsIdempotent` | Fast | Covered |
| Agent severed mid-reconcile (after create) | Reconnect with a fresh controller adopts the half-created release, no duplicate containers, three strategies | `TestChaosPack_AgentSeveredMidReconcile_ReconnectAdoptsHalfCreatedRelease` | Fast | Covered |
| Agent node marked unreachable | Node flips to offline exactly once across repeated passes, placements are never deleted, a fresh heartbeat reports healthy with no write | `TestChaosPack_AgentSevered_NodeMarkedOfflineOnceAndPlacementsKept` (`internal/reconcile/nodehealth`) | Fast | Covered |
| Severed agent's services skipped, then resumed | Resolve returns `ErrNodeNotRegistered` (the pass skips, never tears down), local node unaffected, reconnect resolves the new transport | `TestChaosPack_AgentSeveredThenReconnected_TransportSkippedThenResolves` (`cmd/levelrail`) | Fast | Covered |
| Agent session stream severed | Session ends with an error, no container touched, next session serves | `TestChaos_AgentConnectionSeveredMidStream_ReconnectsWithoutTouchingContainers` (`internal/agent`) | Fast | Covered |
| Control plane killed between desired state and create | New controller converges from any leftover, second pass creates nothing | `TestChaos_ControlPlaneKilledBetweenDesiredStateAndCreate_ConvergesOnRestart` | Fast | Covered |
| Corrupt or missing image at deploy | Typed (`image_corrupt` or `image_pull_failed`), serving release untouched, failed release not pinned, earlier pin kept, converges after repair, pins unchanged by a no-op | `TestChaosPack_CorruptOrMissingImageAtDeploy_TypedAndPreviousStaysPinned` | Fast | Covered |
| Corrupt image loaded by a build | Typed `image_corrupt`, nothing saved | `TestChaosPack_CorruptImageAtLoad_TypedAndNothingSaved` (`internal/deploy`) | Fast | Covered |
| Previous release stays pinned against GC | Serving image and its rollback pin are never in the GC plan, even with zero retention and zero minimum age | `TestChaosPack_FailedDeploy_ServingReleaseAndPinSurviveAggressiveGC` (`internal/imagelife`) | Fast | Covered |
| Corrupt or missing image at rollback | Current release keeps serving, no stop or remove | `TestChaos_CorruptOrMissingImageAtRollback_CurrentKeepsServing` | Fast | Covered |
| OOM during readiness, single replica | Reason `OOMKilledDuringReadiness`, fast fail, old release serving | `TestChaos_OOMKillDuringReadinessWait_OldKeepsServing` | Fast | Covered |
| OOM during readiness, fixed on the next pass | Old release serving after the OOM, the next pass promotes the replacement, then a no-op | `TestChaosPack_OOMDuringReadiness_NextPassAfterFixPromotesAndIsThenNoop` | Fast | Covered |
| OOM on the second replica (half succeeded) | Service never left with zero running containers, typed reason, converges to both new replicas, then a no-op | `TestChaosPack_OOMOnSecondReplicaDuringReadiness_KeepsServingAndConverges` | Fast | Covered |

## Known gaps

None found by this pack. These are limits of what the tests prove, not failing tests:

- The two live tests inject the daemon and disk faults at the runtime boundary on top of a real daemon. They do not stop `dockerd` or fill a real filesystem, because that would take down the nightly host.
- Agent severing is exercised with a fake transport and a fake gRPC stream, not a real mTLS connection. The real-agent reconnect path is measured in `test/e2e/reconcile` (`break_glass_agent_reconnect_test.go`).
- Disk full is proven at the points a deploy writes (pull, start, volume, build, log, metric). A full disk under the Caddy certificate store or the backup writer has its own guards and is not part of this pack.
