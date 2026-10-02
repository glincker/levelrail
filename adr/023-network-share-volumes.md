# ADR 023: NFS/CIFS network shares as a volume backend

Status: Accepted

Date: 2026-10-01

## Context

Operators running Levelrail on a home lab or small office box commonly already have a NAS (Synology, TrueNAS, a plain Linux box exporting NFS) and want app volumes to live there instead of on local disk. A repo-wide grep for NFS/CIFS/SMB/NAS found no existing references, so this is new ground, not a rename of something else.

## Decision

**Use Docker's own `local` volume driver, not a custom mount path.** Docker's built-in driver already understands `--opt type=nfs` and `--opt type=cifs` and shells out to the host's own `mount.nfs`/`mount.cifs` helpers at mount time. `internal/docker.NetworkShareDriverOpts` translates a share's protocol/host/remote path/options into the exact driver-opts map the Engine API's `VolumeCreate` call needs, so this still never shells out to the `docker` CLI, matching every other `internal/docker` method.

**A `network_shares` table, not a special case of `backup_targets`.** A share is a volume source, not a credential-plus-options row on an existing resource (the inverse question ADR 019 answered for storage): no existing table's shape fits. CIFS credentials follow the established split, encrypted via `internal/secrets` under `network-share/<id>`, never a plaintext column. NFS shares write no secret at all, since NFS authenticates by export rules/source IP, not a username and password.

**Test-mount is a reachability dial, not an authentication check.** POST `/api/v1/network-shares/{id}/test` dials the share's host on its protocol's standard port (2049 for NFS, 445 for CIFS) using the same `doctorDialContextOrDefault` seam the registry-reachability doctor check already uses. It deliberately does not attempt a real mount or SMB handshake: a closed port fails any later mount regardless of credential correctness, and a real mount would need a privileged helper process this control plane does not have.

**A new doctor check, gated on actual usage.** `doctorCheckNASClientTools` only runs when at least one configured share actually uses that protocol, the same "only probe what's configured" shape `doctorCheckRegistryReachability` already establishes for registry hosts. A node that has never been asked to mount a network share has no reason to warn about `mount.nfs`/`mount.cifs` being absent.

**Scope stops at CRUD and the driver-opts primitive, not full attach-to-app wiring.** A sibling branch (`feat/volume-attach-ui`, not yet merged as of this writing) owns the app-facing volume-attach flow (`store.ServiceVolume`, `app.yaml`'s `volumes:` key, the reconciler's own volume creation path). This feature ships a complete, standalone Settings page to manage shares and the `docker.EnsureNetworkVolume` primitive that attach work will call; it does not itself add a `network_share_id` to `ServiceVolume` or touch the reconciler, to avoid landing two uncoordinated changes to the same reconciler-adjacent surface.

## Rejected alternatives

- **Shelling out to `mount`/`showmount`/`smbclient` directly.** Violates the project's own Docker-Engine-API-only rule and duplicates what Docker's `local` driver already does correctly.
- **A real authenticated test-mount (actually mounting the share).** Needs a privileged mount helper invocation from the control plane process itself, a much larger surface for a feature whose real validation already happens for free the moment a deploy tries to create the volume.
- **Folding network shares into `backup_targets`.** A backup target is an S3-compatible object store; a network share is a block/file mount target for a running container. Different shape, different consumer (the reconciler's volume creation vs. the backup runner's upload path).
- **Wiring the full attach-to-app flow now.** Risks a collision with the in-flight `feat/volume-attach-ui` branch's own `ServiceVolume` changes; cross-linking is called out explicitly as a follow-up once that branch lands.

## Consequences

Operators can connect a NAS once, under Settings, and reuse it across shares and (once the sibling branch lands) app volumes. The `local`-driver approach means no new runtime dependency on the control plane side; the kernel NFS/CIFS client on each node is the only new requirement, now surfaced by a doctor check instead of a failed deploy.
