---
description: Rotate the envelope-encryption master key without losing access to stored secrets or causing downtime.
---

# Master key rotation

All secrets (app env vars marked `secret: true`, email credentials, tokens, etc.) are encrypted with envelope encryption. Each owner (an app, a backup target, the email settings) gets its own random data encryption key (DEK), and every DEK is wrapped under a master key held in memory. Rotating the master key means re-wrapping every DEK under a new key without ever exposing plaintext secrets.

## When to rotate

- You suspect the master key (the `APP_MASTER_KEY` env var, or the
  `master.key` file in your data directory) was exposed.
- As routine hygiene. `levelrail-cli doctor` surfaces a `master_key_rotation`
  check with how long it has been since the last rotation; this is a soft
  nudge, not an error, and never fails the doctor's overall status.
- Before moving to an external KMS-backed master key in a future release.

::: details What this does and does not require

Rotation runs live against the running control plane with zero downtime. The CLI calls an admin-only endpoint that re-wraps every DEK in a single database transaction. If any DEK fails to unwrap, the entire rotation aborts with no changes.

While a rotation is in progress, all other secret reads and writes are blocked until it completes. This prevents mixing old and new keys.

However, this is not a fully unattended operation due to one constraint: the control plane loads its master key at startup and never reloads it. A successful rotation immediately updates the running process's key, so it serves correctly right away. What happens on the next restart depends on where that key came from:

**File-sourced** (default, no `APP_MASTER_KEY` set):
Rotation automatically rewrites the file at `<data dir>/master.key` with the new key (atomically, so a crash never leaves a truncated file). On restart, the process picks up the new key with no further action.

**Env-sourced** (`APP_MASTER_KEY` set):
Rotation cannot rewrite an environment variable belonging to your process supervisor. The CLI output includes an explicit warning to update `APP_MASTER_KEY` in your systemd unit or Docker Compose file before the next restart. Restarting with the old value will make every secret permanently unreadable, since all DEKs are now wrapped only under the new key.

Always read the CLI's output (or the JSON response's `warning`/`persistedToFile` fields). A silently-skipped warning can cause failure at the next restart, the worst kind of problem to discover.
:::

## How to rotate

1. Generate a new master key. Any valid `filippo.io/age` identity works. The simplest way: let the control plane generate one in a throwaway data directory, or use any tool that emits an age identity.

2. Save the key to a file with mode `600`. Never paste it on the command line, as it will leak into shell history and process listings.
3. Run:

   ```sh
   levelrail-cli secrets rotate-master-key --new-key-file /path/to/new.key
   ```

   Or pipe it through stdin instead of a file:

   ```sh
   cat /path/to/new.key | levelrail-cli secrets rotate-master-key --new-key-file -
   ```

4. Read the output carefully. You will see `rotated_at` and `persisted_to_file` fields. If a `WARNING` line appears, act on it immediately. Update `APP_MASTER_KEY` in your systemd unit or Docker config before the next restart. A `rebound` line reports the slot-binding pass that runs right after the rotation (see [Binding secrets to their slot](#binding-secrets-to-their-slot)).

5. Securely delete the temporary key file once you've confirmed the rotation succeeded (or keep it safe if you still need to update `APP_MASTER_KEY` manually).

This command requires an API token or session with the `root` ability. It's gated the same way as other fleet-wide, irreversible actions like `system prune`, because narrower, per-app-scoped tokens should never reach such powerful operations.

## Failure modes and what they mean

::: details "rotate master key: ... unwrap DEK for ...: ..."
A stored DEK could not be unwrapped with the control plane's current master key. This suggests data corruption or that the running process does not hold the key you expected. Nothing was changed, so it is safe to investigate and retry.
:::

::: details `persistedToFile: false` with a warning about the key file
The rotation itself succeeded (all DEKs are now wrapped under the new key and in use), but writing the new key to `master.key` failed. Usually a permissions or disk-space problem. Fix it and copy the new key into place manually before the control plane restarts.
:::

::: details `persistedToFile: false` with a warning about `APP_MASTER_KEY`
Not a failure. This is the expected message when the master key is env-sourced (see above). It is the required follow-up step, not an error.
:::

## Binding secrets to their slot

Every secret value is stored in a slot: an owner (an app name, or an internal owner such as a backup target) and a key name (`DATABASE_URL`). Values are encrypted with the slot sealed inside, and every read checks it. A ciphertext copied into another slot, for example by someone with write access to the database moving one app's `DATABASE_URL` row into another app's `API_KEY` row, fails to read with a "bound to a different slot" error instead of decrypting in the wrong place.

Values written by releases before this check existed carry no slot and are called legacy. They still decrypt normally, but they are not protected against being copied. Bind them once:

```sh
levelrail-cli secrets binding-status   # total, bound, legacy
levelrail-cli secrets rebind           # bind every legacy value
```

The same status and a **Bind now** button are on **Settings > General** under **Secret slot binding**, and `levelrail-cli doctor` warns with a `secret_binding` check while legacy values remain. A master key rotation also runs a rebind right after it commits, so rotating once binds everything as a side effect.

What `rebind` guarantees:

- **Idempotent.** Already-bound values are checked and left alone. Running it again when nothing is legacy changes nothing.
- **Resumable.** Each value is rewritten on its own, and only if it has not changed since it was read. If a run stops partway (a restart, a disk error), the values it finished stay bound and the next run picks up the rest.
- **Never clobbers a write.** A value someone sets while the rebind runs is reported as `changed` and kept; new writes are always bound.
- **Never launders a swap.** A bound value that is already in the wrong slot is reported under `failed` and not touched. A value whose owner has no data encryption key, or whose key does not unwrap, is reported the same way.
- **Does not bump a secret's age.** Re-encrypting the same value is not a rotation of it, so the stale-secrets check is unaffected.

Rebinding cannot tell whether a legacy value was already swapped before it ran: it binds whatever is in each slot at that moment. If you suspect database tampering, set the affected secrets again rather than rebinding them.

Once `binding-status` reports `legacy: 0`, you can set `APP_SECRETS_REQUIRE_BOUND=true` on the control plane. Reads then refuse any legacy value, so a ciphertext copied in from an old backup cannot be used either. Leave it unset (the default) until the rebind is complete, or unbound secrets will stop resolving.

`POST /api/v1/system/secrets/rebind` requires the `root` ability, like rotation. `GET /api/v1/system/secrets/binding` only needs `read`. Neither ever returns a secret value.

## See also

- [Identity and access](identity-and-access.md) for token abilities and admin access
- [CLI reference](cli-reference.md) for the `secrets` command group
- [Deploying apps](deploying-apps.md) for how app secrets work
- [Installing](installing.md) for configuring `APP_MASTER_KEY` at install time
