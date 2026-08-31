# Master key rotation

Every secret this control plane stores (app env vars marked `secret: true`,
email SMTP credentials, Cloudflare tokens, git provider app secrets, backup
target credentials, and more) is encrypted with envelope encryption: each
service gets its own random data encryption key (DEK), and every DEK is
itself encrypted ("wrapped") under one master key the control plane holds
in memory. Rotating the master key means re-wrapping every stored DEK under
a new master key, without ever exposing a plaintext secret value in the
process.

## When to rotate

- You suspect the master key (the `APP_MASTER_KEY` env var, or the
  `master.key` file in your data directory) was exposed.
- As routine hygiene. `levelrail-cli doctor` surfaces a `master_key_rotation`
  check with how long it has been since the last rotation; this is a soft
  nudge, not an error, and never fails the doctor's overall status.
- Before moving to an external KMS-backed master key in a future release.

## What this does and does not require

Rotation runs **live**, against a running control plane, with no downtime:
`levelrail-cli secrets rotate-master-key` calls an admin-only HTTP endpoint
that re-wraps every stored DEK from the old key to the new one inside a
single database transaction. If any row fails to unwrap (a corrupt DEK, or
the control plane's own currently-held key not matching what you expected),
the whole rotation aborts and nothing changes: there is no partially
rotated state. While a rotation is in progress, any other secret read or
write is blocked until it completes (not racing against it), so a deploy
that reads a secret mid-rotation either sees the old key's results or waits
briefly for the new one, never a mix of the two.

This is *not* a fully unattended, zero-follow-up operation, because of one
real constraint: the control plane loads its master key once at process
startup and never reloads it. A successful rotation immediately updates the
running process's in-memory key, so it keeps serving correctly right away.
What happens on the *next restart* depends on where that key came from:

- **File-sourced** (the default: no `APP_MASTER_KEY` set, key persisted at
  `<data dir>/master.key`): rotation automatically rewrites that file with
  the new key, atomically (write-then-rename, so a crash mid-write can
  never leave a truncated file). A restart picks up the new key with no
  further action.
- **Env-sourced** (`APP_MASTER_KEY` set): rotation cannot rewrite an
  environment variable belonging to your process supervisor. The CLI's
  output includes an explicit warning telling you to update
  `APP_MASTER_KEY` to the new value in your systemd unit, Docker Compose
  file, or wherever it's set, before the control plane is next restarted.
  Restarting with the old value still set will make every stored secret
  permanently undecryptable, since the database now holds every DEK
  wrapped only under the new key.

Read the CLI's output (or the JSON response's `warning`/`persistedToFile`
fields) every time you rotate. A silently-skipped warning here is exactly
the kind of failure this project's own design principles call out as the
most dangerous kind: one that surfaces at the next restart, not at the
moment the mistake was made.

## How to rotate

1. Generate or otherwise obtain a new master key. Any valid
   `filippo.io/age` identity string works; the simplest way to get one is
   to let the control plane generate it for a throwaway data directory, or
   use any tool that emits an age identity.
2. Save the new key to a file you control, readable only by you
   (`chmod 600`). Never paste it on the command line: it would leak into
   shell history and process listings.
3. Run:

   ```sh
   levelrail-cli secrets rotate-master-key --new-key-file /path/to/new.key
   ```

   Or pipe it through stdin instead of a file:

   ```sh
   cat /path/to/new.key | levelrail-cli secrets rotate-master-key --new-key-file -
   ```

4. Read the output. On success you'll see `rotated_at`,
   `persisted_to_file`, and, if applicable, a `WARNING` line. Act on the
   warning immediately if one appears: update `APP_MASTER_KEY` in whatever
   manages your control plane's environment, before the next restart.
5. Securely delete the temporary key file once you've confirmed the
   rotation succeeded (or keep it somewhere safe if `APP_MASTER_KEY` still
   needs updating out of band).

This command requires an API token or session with the `root` ability: the
same tier gated on fleet-wide, no-undo actions like `system prune`, since a
master key rotation is exactly the kind of action a narrower, per-app-scoped
token should never be able to reach.

## Failure modes and what they mean

- **"rotate master key: ... unwrap DEK for ...: ..."**: a stored DEK could
  not be unwrapped under the control plane's currently active master key.
  This should not happen in normal operation; it indicates either data
  corruption or that the running process is not actually holding the key
  you think it is. Nothing was changed: safe to investigate and retry.
- **`persistedToFile: false` with a warning about the key file**: the
  rotation itself succeeded (every DEK is now wrapped under the new key,
  and the running process is using it), but writing the new key to
  `master.key` failed, most likely a permissions or disk-space problem on
  the data directory. Fix that and copy the new key into place by hand
  before the control plane restarts.
- **`persistedToFile: false` with a warning about `APP_MASTER_KEY`**: not a
  failure. This is the expected message whenever the master key is
  env-sourced; it's the required follow-up described above, not an error.
