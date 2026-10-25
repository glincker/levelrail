# ADR 043: Concurrent per-app reconcile on a keyed worker pool

Status: Accepted

Date: 2026-10-25

## Context

`reconcile.Engine.ReconcileAll` runs every controller one after another. An app
whose new container never passes readiness holds the pass for the whole
readiness wait, so every other app, database and the ingress controller wait
behind it. In QA a new app got its containers only after about three minutes
on a single node and over two minutes on multi-node while one app was failing.

A bounded pool inside `ReconcileAll` was tried in v0.8.5 and reverted. It had
three problems:

- The pass still waited for every app (`wg.Wait`), so a stuck app still held
  the next pass, and a new app created meanwhile still waited for it.
- It defaulted to 4 workers, changing behavior for every install at once.
- It ran with no per-app bound, no shutdown story and no ordering tests for
  multi-node, so the release could not show it was safe.

## Decision

Per-service application controllers run on a keyed worker pool that outlives a
single pass. Everything else keeps the sequential path.

- `APP_RECONCILE_WORKERS` sets the pool size. The default is 1, and at 1 the
  pool is never started: `ReconcileAll` runs the exact same sequential loop as
  before. The recommended value for more than a handful of apps is 4, or 8 on
  hosts with many apps. The value is capped at 64.
- The engine is told which controller names are keyed
  (`Engine.SetKeyed(application.IsServiceControllerName)`). That matches
  `application/<service>` only, which covers the app controller, the
  pending-attempt resolver wrapper (`application.WithPendingAttemptResolution`)
  and the node-offline stand-in, since all three share the service's name. The
  delete finalizer, network cleanup, databases, nodes, ingress, mesh, firewall
  and every fixed controller stay inline and in their original order.
- A pass enqueues each keyed controller and returns without waiting for them.
  The next Docker event, nudge or resync tick starts the next pass as before.

### Per-key serialization

The queue key is `Controller.Name()`. At most one run per key is in flight.

- Added while queued: the key keeps its place in the queue and takes the newer
  controller instance (fresher desired state; level-triggered, so the older one
  is redundant).
- Added while in flight: the key is queued once more and runs after the current
  run ends, never alongside it.
- So each key has at most one run in flight and one waiting, whatever the event
  rate. Queue depth is bounded by the number of apps.

### Fairness

Workers take the oldest queued key that is not in flight. A key that runs
often, for example one in a requeue loop, goes to the back each time it is
added after starting, so it cannot starve other apps.

### Failure and backoff isolation

The existing per-name breaker (`Guard`) and the per-name requeue streak
(`APP_RECONCILE_REQUEUE_MAX`) are unchanged and already keyed by name. A
failing app opens only its own breaker and doubles only its own requeue delay.
With the pool, its slow or failing run also occupies only one worker, so it
delays only itself.

### Bounded blocking waits

Every keyed run gets a context with `APP_RECONCILE_KEY_TIMEOUT` (default 10m),
so a readiness wait, image pull or hung runtime call cannot hold a worker
forever. Controllers are already required to be safe to interrupt; a run cut at
the limit records `context.DeadlineExceeded` as its error, counts against its
breaker and is retried on the next pass. The limit only applies with more than
one worker, so the default path is unchanged.

Readiness waits still block the worker for their own duration. Moving them off
the worker (return `RequeueAfter` instead of waiting) is a later slice; with N
workers, N apps stuck at once can still delay the rest, which is why the
recommended value is 4 or more.

### Deleted apps

At the end of each successful pass, keys missing from the live set are dropped
from the queue and their in-flight run is cancelled, so a deleted app does not
keep recreating containers while the delete finalizer and network cleanup run.
A Source error skips this step, as it already skips the whole dynamic pass.

### Ordering

Apps were already reconciled before databases in each pass, so no app relied on
a database converging earlier in the same pass. Apps that depend on a database
or on each other converge level-triggered over later passes, as before. What
changes with workers above 1 is that ingress may run before an app's new
container exists; the container start event triggers the next pass, which
routes it.

The pass snapshot (`APP_RECONCILE_SNAPSHOT_MAX_AGE`) bounds how stale a queued
controller's prefetched container list can be, so a run that waited in the
queue reads live state.

### Shutdown

When `Run`'s context ends the pool stops taking work, drops queued keys, lets
in-flight runs finish for at most the shutdown drain (`APP_SHUTDOWN_DRAIN`, the
same limit the sequential pass already used), then cancels them and waits for
every worker to exit before `Run` returns. No worker goroutine outlives `Run`.

### Metrics

`Engine.Stats` gains an optional `queue` object, present only when the pool is
running: workers, queue depth, in flight count, and the oldest queued key's
wait. Pass timing keeps its meaning for the inline controllers.

## Failure modes

- A controller that ignores its context still holds its worker past the key
  limit. All runtime calls in the app controller take a context, so this needs
  a bug, and it shows up as `in_flight` stuck at a value with `oldest_wait`
  growing.
- Status rows for a run cut at the key limit are written with the cut context
  and may fail; the next run writes them.
- Two keyed controllers that share a non-keyed resource (a per-app network)
  could race on it. Both use idempotent create-by-name, and the network cleanup
  controller removes only networks whose app row is gone, after a grace period.

## Rollout

1. v0.8.6 ships with the default of 1, byte-for-byte the old behavior.
2. QA runs one droplet with `APP_RECONCILE_WORKERS=4`, one app that never
   passes readiness and at least ten healthy apps, and checks that new and
   changed healthy apps converge within a normal pass time.
3. Same on a two-node install, including a node going offline mid-deploy.
4. If both hold, the default moves to 4 in a later release.

Later slices: readiness waits as requeues, databases and ingress on the same
queue, per-node fairness, and the queue metrics on the health page.
