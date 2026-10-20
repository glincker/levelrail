# Fix actions: buttons first, commands second

When the dashboard finds a problem, the primary control is a button. A CLI command may appear only inside a collapsed "Fix via CLI" disclosure next to that button.

## The pattern

One component family in `web/src/components/fixit/`:

| Piece | Role |
| --- | --- |
| `FixAction` | A typed fix from `internal/fixit`. Primary button is "Fix now" (auto, with the change preview and confirm dialog) or "Re-check" (guided and manual, the step happens elsewhere). Result line shows fixed, still failing with the reason, or cannot fix here. |
| `ManualFix` | Same anatomy for a check with no typed fix: "Re-check" first, prose guidance as text, any command collapsed. |
| `FixCli` | The collapsed "Fix via CLI" disclosure holding one or more commands, each with a copy button. Collapsed by default. |
| `CheckFix` / `CheckDetail` | Look up the fix for a failing check id and render one of the above. |

Auto-apply on page load (`autoApply`) runs only when all of these hold: the fix is `auto`, risk is `safe`, it is reversible, its state is `offer` (not demoted or rate limited), it takes no parameter, the viewer is root, and this subject has not been auto-applied yet in this browser session. The apply goes through `POST /api/v1/fixes/{id}/apply`, so it is audited and recorded in the attempt memory like a manual click. Restarts and exposes fixes are never auto-applied.

Server side, a fix is registered in `internal/fixit` with a preview (`Changes`), a risk, a re-check (`Detect` runs again after `Run`), and the engine's attempt memory and hourly cap. Host-level steps the control plane cannot perform use `GuidedCheckFix`: the engine only re-checks, and the host command is shown as the secondary option.

## Inventory

Treatments: Auto (button does the work), Guided (Re-check button, command collapsed), Validate+CLI (re-check or existing action first, commands collapsed), Exception (nothing in the dashboard can run, kept as terminal content).

| Surface | What it asked the operator to run | Can the control plane do it? | Treatment |
| --- | --- | --- | --- |
| Doctor: control plane backup (`control_plane_backup`) | `control-plane-backups create` | Yes, it owns the backup manager | Auto, safe, reversible; new fix `backup.control_plane.snapshot`; applies on load once per session |
| Doctor: firewall (`firewall`) | `sudo ufw allow` / `firewall-cmd` / `iptables` | No, host firewall, and the process may be in a container | Guided; new fix `host.firewall.open_web_ports` (risk exposes, Re-check only), command collapsed |
| Doctor: any other check with a `fix` string (`CheckDetail`) | Command or prose in the fix field | Per check, see rows below | Prose shown as text; a command goes in `FixCli`; "Re-check" re-runs the doctor |
| Mesh: public address, WireGuard privileges, UDP port, disk, network pool | Edit env, restart, free a port | Public address, disk, network pool: yes. Privileges and port holder: no | Already typed fixes; guided command moved into `FixCli` |
| Doctor: NTP / clock skew | Install an NTP client | No, host package | ManualFix: prose + Re-check (no detect-per-list network call) |
| Doctor: ingress survives restarts (`LEVELRAIL_SOCKET_ACTIVATION=1 ./install.sh upgrade`) | Re-run the installer | No, host install | ManualFix: Re-check, command collapsed |
| Doctor: published ports (`firewall exposure` / `firewall restrict`) | CLI firewall restrict | Partly (restrict is a firewall write, risk exposes) | Left as ManualFix; a typed restrict fix is future work |
| Doctor: cross-node ingress | `nodes mesh` / `apps set-node` | Partly (placement change restarts the app) | ManualFix with collapsed CLI |
| Doctor: NAS client tools, registry egress, ACME egress, advertise host reachability | Install a package, open egress, set `APP_AGENT_ADVERTISE_HOST` | No for packages and egress; advertise host has the mesh public-address fix | ManualFix: prose + Re-check. Advertise host check is not yet linked to the mesh fix (different check id) |
| Doctor: resources (RAM, CPU, disk latency) | Prose advice | No | Prose + Re-check |
| Server readiness (`ServerReadiness`) | `next_step` command | No, host level | "Re-check" form button is primary; `next_step` collapsed in `FixCli` |
| Edge update guidance (`EdgeUpdateGuidance`) | `docker pull`, recreate, compose snippet | No, the control plane cannot recreate its own container | Channel switch buttons stay primary; pull, recreate and compose collapsed in `FixCli` |
| Rollback plan dialog (`RollbackPlanDialog`) | fetch, apply, restore commands | No, binary swap on the host | "Re-check" plan button in the footer; commands collapsed in `FixCli` |
| Device login resolved (`ResolvedLoginControls`) | `levelrail-cli auth login --device` | No, the login starts in the CLI | Dismiss and audit buttons stay; command collapsed in `FixCli` |
| App import preflight disk check | "run du -sh" | No, source host | Message rewritten to state the problem; re-run preflight |
| Cutover readiness (`internal/cutover/readiness.go`) | Link actions to app pages | Already buttons/links | No change |
| Domain doctor (`internal/domaindoctor`) | Mostly request/link actions | Already buttons (Start app, Renew, links); DNS records are at the operator's provider | No change |
| Failure classifier (`internal/failure/failure.go`) | Prose suggested fix | n/a | No change |
| Add node dialog (`AddNodeDialog`) | Join command | No, runs on the new machine | Exception: the command is the enrollment itself; the dialog already detects the node arriving |
| Locked-out dialog, register form setup token | `recover-admin`, `emergency-token`, `setup-token` | No, the operator cannot sign in | Exception: terminal is the only surface |
| CLI access page (`cli-access.tsx`) | Install and auth commands | n/a, this page is about the CLI | Exception: commands are the content |
| Badge settings card | Comment only | n/a | No change |

## Left for the upgrade branch

`UpgradeCheckRow.tsx`, `UpgradePreflight.tsx`, `UpgradeNow.tsx`, `kit/upgrade/*`, `internal/api/updates_preflight.go` and `internal/api/doctor_release_signature.go` still emit `sudo ... self-upgrade` / `install-cosign` commands and were not touched.

## CLI parity

The CLI keeps printing commands. Only dashboard presentation changed; no CLI command or output was altered.
