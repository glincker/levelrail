---
title: Server patching
description: Preview and install OS package updates on a server, with holds for kernel and container runtime upgrades.
---

# Server patching

`nodes patch-status` reports how many OS package updates a server has pending. `nodes patch preview` and `nodes patch apply` install them safely.

**Status: beta.** Read these limits before using it:

- **Only the control plane's own host (`local`) is supported.** Every other node answers `501 patch_unsupported` until an agent exec path exists; patch those on the node itself.
- **A containerized control plane cannot patch its host.** It has no apt, dnf or yum of the host, so preview returns `501 patch_manager_unsupported`. Use the standard systemd install.
- **Kernel, docker, containerd, runc and moby upgrades need systemd.** The upgrade then runs as its own transient unit (`systemd-run`), outside the control plane's service, so a docker restart cannot interrupt dpkg. When the control plane is not running under systemd, or `/usr/bin/systemd-run` is missing, these packages are refused with `409 runtime_upgrade_needs_systemd` even with `--include-all`; ordinary security packages still apply.
- **Rebooting `local` takes the control plane and the dashboard down** until the host is back.

Supported package managers: apt (Debian, Ubuntu), dnf and yum (Fedora, RHEL family), detected in the same order the patch status check uses. Commands run with absolute paths and a minimal environment (fixed `PATH`, `LANG=C`, `DEBIAN_FRONTEND=noninteractive`, `APT_LISTCHANGES_FRONTEND=none`, `NEEDRESTART_MODE=l`, `NEEDRESTART_SUSPEND=1`), so needrestart only lists services and never restarts docker.

## Preview

```sh
levelrail-cli nodes patch preview local
levelrail-cli nodes patch preview local --include-all --refresh
```

A preview changes nothing on the host. It runs a simulation (`apt-get -s upgrade`, or `dnf check-update` and `dnf check-update --security`) and lists exactly the packages an apply would install, each flagged as `security`, `kernel` or `container runtime`.

- By default only security updates are listed.
- Kernel, docker, containerd, runc and moby packages are **held** unless you pass `--include-all`, because they restart every container or need a reboot.
- `--refresh` runs `apt-get update` (or `--refresh` for dnf) first.
- The preview returns a `preview_id` that expires after `nodes.patch_preview_ttl` (default 10 minutes).

## Apply

```sh
levelrail-cli nodes patch apply local --yes
levelrail-cli nodes patch apply local --yes --include-all --reboot-if-required
```

Without `--yes`, apply only prints the preview. With it, the control plane:

1. Refuses if another patch run is active on the node (`patch_in_progress`) or a deploy is in progress (`deploy_in_progress`).
2. Refuses if the preview is unknown or expired (`preview_expired`), or if `--include-all` differs from the preview (`preview_mismatch`).
3. Simulates again and refuses if the package list changed since the preview (`preview_stale`). Each preview id is single use.
4. With `--reboot-if-required`, refuses up front when apps or databases run on the node (`drain_required`): drain the node first.
5. Installs exactly the previewed packages pinned to their previewed versions (`apt-get install --only-upgrade --no-remove name=version ...`, or `dnf upgrade name-version.arch ...`), as a fixed argument list with no shell, and writes an audit entry with the package and held lists. Under systemd the install runs as a transient unit; otherwise it runs as a child process.
6. When `nodes.patch_apply_timeout` passes, the control plane stops waiting and records the run as `timed_out`; it never kills the package manager, which keeps running.
7. Keeps the last `nodes.patch_output_tail_lines` lines of output with secret looking values masked, then reports the new pending counts.

It never reboots unless `--reboot-if-required` is set and the node still has no workloads and no deploy when the upgrade ends; the reboot is scheduled one minute out. Otherwise the run says a reboot is required.

## Run history

```sh
levelrail-cli nodes patch runs local
```

The last `nodes.patch_run_history` runs per node (default 20). History is kept in memory and resets when the control plane restarts; the audit log keeps the permanent record (a start and a finish entry per run, with status and exit code). If the control plane restarts during a run, the CLI reports the outcome as unknown; check `nodes patch runs` and `nodes patch-status`.

## API

All three routes need the root ability.

| Route | Body | Returns |
| --- | --- | --- |
| `POST /api/v1/nodes/{id}/patch/preview` | `{"include_all": false, "refresh": false}` | preview: `packages`, `held`, `other_available`, `reboot_required_now`, `reboot_likely`, `preview_id`, `expires_at` |
| `POST /api/v1/nodes/{id}/patch/apply` | `{"preview_id": "...", "confirm": true, "include_all": false, "reboot_if_required": false}` | `202` with the run (status `running`) |
| `GET /api/v1/nodes/{id}/patch/runs` | none | runs, newest first: `status`, `packages`, `held`, `exit_code`, `output_tail`, `reboot_required`, `reboot_scheduled`, `reboot_note`, `patch_status_after` |

## Settings

| Key | Default | Meaning |
| --- | --- | --- |
| `nodes.patch_preview_ttl` | `10m` | How long a preview id stays valid |
| `nodes.patch_apply_timeout` | `30m` | How long to wait for an upgrade; on timeout it is left running and the run is recorded as `timed_out` |
| `nodes.patch_output_tail_lines` | `200` | Output lines kept per run |
| `nodes.patch_run_history` | `20` | Runs listed per node |
| `nodes.patch_poll_interval` | `2s` | How often a detached upgrade unit is checked |
| `nodes.patch_adopt_limit` | `20` | Upgrade units left by a previous control plane process that are adopted again (1 to 200) |

## Known limits

- A docker or containerd upgrade restarts the control plane through its `Requires=docker.service` dependency. The upgrade finishes in its own `<name>-patch-<run>` unit, which records the package list in its description. After a restart the control plane adopts those units (up to `nodes.patch_adopt_limit`) the first time the patch API is used: it rebuilds the run record, waits for a unit that is still running, writes the finish audit entry and only then stops and resets the unit. The start time of an adopted run is the adoption time.
- Run history lives in memory. The one-run-per-node lock follows the upgrade itself: after a timeout, or for an adopted unit, the node stays busy until the unit or child process ends, and a timed out run is updated with its real result (no reboot follows a late success).
- Before installing, the exact install command is simulated (`apt-get -s install`, `dnf` or `yum --assumeno`). If it would install, upgrade or remove any package outside the preview, the apply is refused with `preview_mismatch` and the extra names; a simulation that fails refuses with `simulation_failed`.
- A failed unit, or one whose result is not success, is always recorded as failed, and a reboot only follows a succeeded run.

The control plane must run with permission to use the package manager (root, as the standard install does).
