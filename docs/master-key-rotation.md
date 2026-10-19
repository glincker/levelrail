---
title: "Master key rotation"
description: Rotate the envelope-encryption master key without losing access to stored secrets or causing downtime.
---

All secrets (app env vars marked `secret: true`, email credentials, notification channel webhook URLs and keys, tokens, etc.) are encrypted with envelope encryption. Each owner (an app, a backup target, the email settings) gets its own random data encryption key (DEK), and every DEK is wrapped under a master key held in memory. Rotating the master key means re-wrapping every DEK under a new key without ever exposing plaintext secrets.

## When to rotate

- You suspect the master key (the `APP_MASTER_KEY` env var, or the `master.key` file in your data directory) was exposed.
- As routine hygiene. `levelrail-cli doctor` has a `master_key_rotation` check that reports how long it has been since the last rotation. It is a soft nudge and never fails the doctor's overall status.

<Accordion title="What this does and does not require">


Rotation runs live against the running control plane with zero downtime. The CLI calls an admin-only endpoint that re-wraps every DEK in a single database transaction. If any DEK fails to unwrap, the entire rotation aborts with no changes.

While a rotation is in progress, all other secret reads and writes are blocked until it completes. This prevents mixing old and new keys.

However, this is not a fully unattended operation due to one constraint: the control plane loads its master key at startup and never reloads it. A successful rotation immediately updates the running process's key, so it serves correctly right away. What happens on the next restart depends on where that key came from:

**File-sourced** (default, no `APP_MASTER_KEY` set). The file is `<data dir>/master.key`, or the path in `APP_MASTER_KEY_FILE` if you set it. Point `APP_MASTER_KEY_FILE` at an absolute path outside the data directory (for example `/etc/levelrail/master.key`, mode `0600`) so a copy of the data directory alone cannot decrypt your secrets. A file with group or world permissions is refused at startup.

Rotation automatically rewrites the file at `<data dir>/master.key` with the new key (atomically, so a crash never leaves a truncated file). On restart, the process picks up the new key with no further action. The new key is written to `master.key.new` (fsynced) before the database is touched, and renamed over `master.key` only after the rotation commits. If the process dies in between, the next start detects `master.key.new`, checks which key your stored secrets actually use, and finishes the swap (or discards the staged file if the rotation never committed), so you never end up with a key file that cannot decrypt your data. If the new key file cannot be staged, the rotation is refused and nothing changes.

**Env-sourced** (`APP_MASTER_KEY` set):
Rotation cannot rewrite an environment variable belonging to your process supervisor. The CLI output includes an explicit warning to update `APP_MASTER_KEY` in your systemd unit or Docker Compose file before the next restart. Restarting with the old value will make every secret permanently unreadable, since all DEKs are now wrapped only under the new key.

Always read the CLI's output (or the JSON response's `warning`/`persistedToFile` fields). A silently-skipped warning can cause failure at the next restart, the worst kind of problem to discover.

</Accordion>

## How to rotate

<Steps>
<Step title="Generate a new master key">

Run this on the machine you run the CLI from:

```sh
levelrail-cli secrets generate-master-key --out /path/to/new.key
```

This writes the key with mode `600` and refuses to overwrite an existing file. The key must be a post-quantum hybrid age identity (it starts with `AGE-SECRET-KEY-PQ-`), which only this command, or a control plane's own `master.key`, produces. A key made with plain `age-keygen` (`AGE-SECRET-KEY-1...`) is rejected with a `400` pointing back at this command, and nothing is changed.

Never paste the key on the command line: it leaks into shell history and process listings. Keep it in the file.

</Step>
<Step title="Rotate">

```sh
levelrail-cli secrets rotate-master-key --new-key-file /path/to/new.key
```

Or pipe the key through stdin:

```sh
cat /path/to/new.key | levelrail-cli secrets rotate-master-key --new-key-file -
```

</Step>
<Step title="Read the output">

You will see `rotated_at` and `persisted_to_file` fields. If a `WARNING` line appears, act on it immediately: update `APP_MASTER_KEY` in your systemd unit or Docker config before the next restart. A `rebound` line reports the slot-binding pass that runs right after the rotation (see [Binding secrets to their slot](#binding-secrets-to-their-slot)).

</Step>
<Step title="Delete the temporary key file">

Do this once you have confirmed the rotation succeeded. Keep it safe instead if you still need to update `APP_MASTER_KEY` by hand.

</Step>
</Steps>

This command requires an API token or session with the `root` ability. It's gated the same way as other fleet-wide, irreversible actions like `system prune`, because narrower, per-app-scoped tokens should never reach such powerful operations.

## Refresh your escrow bundle

If you keep a [key escrow bundle](/disaster-recovery#after-rotating-the-master-key), it still holds the old key after a rotation. Generate a new one with `levelrail-cli control-plane-backups escrow` and replace the stored copy. For an env-sourced key, update `APP_MASTER_KEY` first.

## Failure modes and what they mean

<Accordion title="&quot;rotate master key: ... unwrap DEK for ...: ...&quot;">

A stored DEK could not be unwrapped with the control plane's current master key. This suggests data corruption or that the running process does not hold the key you expected. Nothing was changed, so it is safe to investigate and retry.

</Accordion>

<Accordion title="`persistedToFile: false` with a warning about the key file">

The rotation itself succeeded (all DEKs are now wrapped under the new key and in use), but the final rename of `master.key.new` over `master.key` failed. Leave `master.key.new` in place: the next start applies it automatically. A failure to write `master.key.new` in the first place refuses the rotation before anything changes.

</Accordion>

<Accordion title="`persistedToFile: false` with a warning about `APP_MASTER_KEY`">

Not a failure. This is the expected message when the master key is env-sourced (see above). It is the required follow-up step, not an error.

</Accordion>

Notification channel credentials are covered by the same rotation: each channel has its own DEK, re-wrapped like every other owner's, so channels keep delivering after a rotation without being edited.

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

## Troubleshooting

### Rotation fails with "unwrap DEK" error

Likely cause: The control plane is not running with the correct current master key, or a data corruption occurred.

How to confirm: Check which key the process is using: `ps aux | grep levelrail | grep APP_MASTER_KEY` shows the env var if set, or `ls -la /var/lib/levelrail-data/master.key` shows the file location. Try decrypting a secret: `levelrail-cli apps env <app> | head -1` should work if the key is correct.

Fix: Verify you are using the correct current master key (the one the control plane is already running with). Do not try to rotate with a new key if the current key is wrong. If a DEK is corrupted, restore the control plane database from a backup before the corruption occurred.

Still stuck: Check control plane logs for key loading errors: `levelrail logs --since 1h | grep -i "master.*key\|unwrap"`.

### Rotation succeeded but process crashed before writing new key to file

Likely cause: A disk or permissions issue prevented the new key from being written, but the rotation in memory succeeded.

How to confirm: Check the CLI output: it says `persistedToFile: false` with a specific error. Check disk space: `df -h / | grep -v Use` shows available space. Check file permissions: `ls -la /var/lib/levelrail-data/master.key` should be readable/writable by the control plane user.

Fix: The rotation is done; secrets are now wrapped under the new key in the database. Manually copy the new key to the correct location with the right permissions: `cp /path/to/new.key /var/lib/levelrail-data/master.key && chmod 600 /var/lib/levelrail-data/master.key`. On next restart, the control plane will pick up the new key.

Still stuck: Do not restart the control plane until the file is in place, or all secrets will become unreadable.

### "APP_MASTER_KEY environment variable not updated" warning after rotation

Likely cause: The master key is env-sourced (set via `APP_MASTER_KEY` variable), and the rotation succeeded, but you have not yet updated the env var in your systemd unit or Docker config.

How to confirm: Check how the key is configured: `ps aux | grep levelrail | grep APP_MASTER_KEY` shows the env var if set. If set, the rotation requires manual follow-up.

Fix: Before the next restart, update `APP_MASTER_KEY` in your systemd unit (`/etc/systemd/system/levelrail.service`) or Docker Compose file (`docker-compose.yml`). Replace the old key value with the new one. You can find the new key in the output of the `rotate-master-key` command or read it from the temporary file you created. After updating, reload and restart: `systemctl daemon-reload && systemctl restart levelrail` (or `docker-compose restart`).

Still stuck: If you restart with the old key, all secrets will become unreadable. Stop and restore from a backup, then update the key before starting again.

### Rebind is taking a long time or appears stuck

Likely cause: Many secrets exist and the rebind is still in progress, or a disk write is slow.

How to confirm: Check the rebind status: `levelrail-cli secrets binding-status` shows progress. Check control plane logs: `levelrail logs --since 5m | grep rebind` shows how many values have been processed.

Fix: No action needed; rebind is safe to interrupt. If you need to stop it, restart the control plane (it will resume where it left off on the next rotation). Rebind is idempotent, so running it multiple times is safe.

Still stuck: If rebind seems truly stuck (no log updates for 10 minutes), check if the control plane is responding: `curl -s http://localhost:8080/api/v1/brand` should return brand info. If not responding, restart the control plane.

### Cannot create new master key, "invalid key format" error

Likely cause: You are trying to use an age key in the wrong format (plain `age-keygen` instead of the post-quantum version).

How to confirm: Check the key: `head -1 /path/to/key.txt` should start with `AGE-SECRET-KEY-PQ-`. If it starts with `AGE-SECRET-KEY-1-`, it is a plain age key, not the post-quantum format needed.

Fix: Generate a new key with the correct command: `levelrail-cli secrets generate-master-key --out /path/to/new.key`. This creates a post-quantum hybrid key in the correct format.

Still stuck: Never create an age key by hand; always use `levelrail-cli secrets generate-master-key`.

### After rotation, a specific secret returns "bound to a different slot" error

Likely cause: That secret is a legacy value (written before slot binding existed), and it was copied to the wrong slot in the database, or data corruption occurred.

How to confirm: Check the binding status: `levelrail-cli secrets binding-status` shows how many legacy values remain. If it is zero, all secrets are bound and none should have this error. Check the specific resource: `levelrail-cli apps env <app> | grep <key>` should work for a correctly bound secret.

Fix: Set the secret again: this creates a new bound value: `levelrail-cli apps secrets set <app> <key> <value>`. Or run rebind to fix legacy values: `levelrail-cli secrets rebind`. If the error persists, the secret may be corrupted; restore from a backup before the corruption occurred.

Still stuck: Check control plane logs for slot errors: `levelrail logs --since 1h | grep "bound to a different slot"`.

## Next steps

<CardGroup cols={2}>
<Card title="Disaster recovery" href="/disaster-recovery">

Refresh your escrow bundle after rotating.

</Card>
<Card title="Identity and access" href="/identity-and-access">

Token abilities and admin access.

</Card>
<Card title="CLI reference" href="/cli-reference">

The full `secrets` command group.

</Card>
<Card title="Installing" href="/installing">

Configure `APP_MASTER_KEY` at install time.

</Card>
</CardGroup>
