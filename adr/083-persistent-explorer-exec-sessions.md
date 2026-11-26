# ADR 083: Persistent exec sessions for the database explorer

Status: Proposed

## Context

Every explorer request starts a fresh client process inside the database container and throws it away. Measured on v1.5.11 (20 sequential calls against the dev API, real demo containers, loaded laptop):

| Route | p50 | p95 | Execs |
| --- | --- | --- | --- |
| Mongo collections overview (2 collections) | 697 ms | 816 ms | 1 |
| Mongo find (limit 25) | 628 ms | 733 ms | 1 |
| Redis key scan (SCAN, then pipelined TYPE and TTL) | 158 ms | 180 ms | 2 |
| SQL schema outline, rows page | not measured, no Postgres or MySQL demo container | | 1 |

`docker exec <c> true` costs about 94 ms. mongosh start plus connect plus auth costs 400 to 550 ms. Flag experiments (`--nodb` with an explicit connect, `--tls=false`, telemetry off, `--host 127.0.0.1`) gave no gain: the cost is Node boot plus the connection, not a setting.

What the code does today (read on `release/v1.5.11`):

- `dbviewer.Execer` (`internal/dbviewer/result.go`) has one method, `ExecWithInput(ctx, containerID, cmd, stdin) (io.ReadCloser, error)`. `docker.Client.ExecWithInput` (`internal/docker/client.go`) creates and attaches an exec, copies stdin, then `CloseWrite`, and streams stdout until the process exits. EOF on stdin is what ends every request. The agent path (`internal/agent`) implements the same method for remote nodes.
- Mongo (`internal/dbviewer/mongo.go`): `sh -c "exec mongosh --quiet --norc --file /dev/stdin"`. The script is built per request from validated parts and JSON literals (`mongoScript`, `jsLit`, `jsParse`); the request is code. It authenticates with `MONGO_INITDB_ROOT_USERNAME` and `_PASSWORD` read from the container's own environment. Output is one JSON document per line plus `LRMORE`, `LRLAST:`, `LRERR:` and a final `LRDONE`. The markers are fixed strings, not per request.
- Postgres (`internal/dbviewer/run.go`): `psql -X -q --no-password --csv -v ON_ERROR_STOP=1` over the local trust socket, `PGOPTIONS` sets `default_transaction_read_only`, `statement_timeout`, `lock_timeout`, `idle_in_transaction_session_timeout`. Reads run inside `BEGIN READ ONLY` with a cursor and `ROLLBACK`. NULL is a random sentinel per request.
- MySQL and MariaDB: `MYSQL_PWD` taken from the container env inside `sh -c`, `--xml`, `MAX_EXECUTION_TIME` or `max_statement_time`, `START TRANSACTION READ ONLY ... ROLLBACK`.
- Redis family (`internal/dbviewer/redis.go`): `redis-cli` (or `keydb-cli`) with a TLS probe, one exec per step.
- The SQL guard (`internal/dbviewer/guard.go`) rejects any backslash and lexes the statement, failing on unterminated quotes, comments and dollar quotes. Mongo operations are an allow list (`mongo_guard.go`).
- Limits are live runtime settings (`internal/runtimesettings/registry_dbviewer.go`, `database_viewer.*`, seeded from `APP_DB_QUERY_*`).
- ADR 082 moves the explorer off the superuser onto a shared read only login (`lr_ro`) and per user brokered logins with an unlock window.

The property to keep: database credentials never enter the control plane for managed databases. The client authenticates inside the container with what the container already has (env, local socket trust), or with a brokered secret that ADR 082 already holds in memory only.

## Decision

Add persistent exec sessions: one long lived client process per (container ID, engine, database login, mode), started once with `docker exec -i`, kept open, and fed framed requests over stdin. Behind a live setting, default OFF in 1.5.11, ON after one release of soak. One shot exec stays as the fallback and as the only path for writes in the first version.

Recommendation, in order of value:

1. MongoDB first, with a small constant loader program run once by mongosh, reading JSON lines. Requests are data, not code. This removes the 400 to 550 ms per request and also removes per request script generation, which is a security gain on its own.
2. Redis family second (two execs become zero).
3. MySQL and MariaDB, then Postgres, last, through the client in batch mode with nonce markers, gated on the guard's lexer.

Scope of a session in v1: reads only (explorer browse, find, count, indexes, schema outline, rows page, console reads). Writes, imports, DDL and anything needing `root` or an unlocked brokered login with a write preset keep one shot exec. This keeps the isolation problem small: a read session can never hold a transaction that commits.

### Session key and identity

```
key = (container ID, engine, login, mode)
login = "container-env" | "lr_ro" | "lr_u_<hash>"   (ADR 082 section 2)
mode  = "read"
```

- The container ID is the full ID, not the name. A replaced container (upgrade, redeploy, restore) has a new ID, so its old sessions can never be found again and are closed by the invalidator.
- Never share a session across logins. A caller resolved to `lr_ro` gets the `lr_ro` session; a caller whose per user setting puts reads on a brokered login gets that user's session. There is no "upgrade" of a session's identity and no `SET ROLE` inside a session.
- A session carries the identity it authenticated as in its ready frame (`whoami`), and the pool checks it against the key before the first request. A mismatch kills the session.
- Brokered sessions close when the brokered login is dropped (sweeper, logout, session revoke, role change, SCIM deprovision), before the drop runs, through a hook the sweeper calls.

### Authentication inside the session

- Container env login (today's model, until ADR 082 M02 lands): the loader reads `MONGO_INITDB_ROOT_*`, `MYSQL_ROOT_PASSWORD`, `MARIADB_ROOT_PASSWORD` from its own process env inside the container, exactly as the one shot scripts do. Postgres uses the local socket. The control plane never sees the value.
- `lr_ro` and brokered logins whose engine needs a password (MySQL, MariaDB, MongoDB, Redis ACL): the secret is sent as the first stdin frame (`auth`), held by the loader in memory, never in process args and never in exec env. ADR 082 says "passed via exec env"; this ADR recommends amending that to the stdin frame, because `/proc/<pid>/environ` is readable by the same user inside the container for the life of a long running process, and exec env is retained in the daemon's exec config. One shot processes had the same exposure for under a second; a session has it for minutes.
- Process args contain only constant code (the loader) and never data.

### Frame protocol

Common rules for every engine:

- Request id: 128 random bits, hex, fresh per request. It doubles as the nonce for the end marker.
- Strict one request in flight per session. The next request is not written until the current one's `DONE` with the matching id arrived, or the session was killed. This is the core anti smuggling rule.
- A response line that names a different id, or a protocol line out of order, poisons the session: kill, fall back to one shot for this request, count `protocol_violation`.
- A missing `DONE` within the request deadline kills the session. A killed session is never reused.
- Output is still read through the existing `capReader` and per cell caps.

MongoDB (loader). Request, one line, produced by Go `json.Marshal` (so no raw newline can appear in a frame):

```
{"v":1,"id":"9f2c...e1","op":"find","db":"shop","coll":"customers","filter":"{\"status\":\"active\"}","sort":"{\"_id\":1}","after":null,"limit":25,"ms":15000,"cell":65536}
```

`filter`, `sort`, `after` are canonical extended JSON strings that the loader passes through `EJSON.parse`, the same values the guard validates today. `op` is matched against a constant table inside the loader; unknown ops return `ERR`.

Response, every line written by the loader, every line prefixed:

```
LR1 9f2c...e1 D {"_id":1,"name":"a"}
LR1 9f2c...e1 MORE
LR1 9f2c...e1 LAST {"_id":{"$oid":"..."}}
LR1 9f2c...e1 ERR "server error text as a JSON string"
LR1 9f2c...e1 DONE 25
```

Documents go through `EJSON.stringify` with no indentation, which escapes newlines and control characters, so a document cannot start a line. A forged `LR1 <id> DONE` inside a value cannot end the frame early because it is never at the start of a line, and the id is unknown to the data author anyway. Ready frame after start and after auth: `LR1 0 READY {"pid":<pid in container>,"user":"<authenticated user>","engine":"<version>"}`. Lines that do not start with `LR1 ` (mongosh warnings) are counted and dropped, never parsed as data.

Loader shape (constant, embedded in the binary, about 150 lines): `mongosh --quiet --norc --nodb --eval "<loader>"`, then inside the loader `connect()` with credentials from env or the auth frame, then a loop that reads stdin lines synchronously with `require("fs").readSync(0, ...)`, dispatches, prints frames, and exits on EOF. VERIFY in slice 3: that blocking `readSync` in a mongosh script keeps the shell API calls working on mongosh 1.x and 2.x, and that `print` flushes per line when stdout is a pipe. If it does not, the fallback design is the REPL fed one wrapped expression per request (heavier, code per request, more state leakage); that fallback is not built unless the spike fails.

Why not the plain REPL: the REPL keeps variables, `use` state, cursors and `db` across inputs, evaluates code (so every request is still generated script), prints banners and prompts, and has no clean frame boundary. The loader keeps none of that: each request runs in a fresh function scope, uses `client.getSiblingDB(name)` explicitly, never calls `use`, closes every cursor in `finally`, and has no global the request can reach.

Redis family: `redis-cli` with no command reads commands from stdin line by line. After each request the session writes `ECHO "LR1:<id>:DONE"` and reads until that exact line. Keys and patterns are quoted by the existing `redisQuote`. Values in `--raw` mode may contain newlines, so the line count limit stays and the nonce marker is what ends the frame. VERIFY: per reply flushing when stdout is a pipe, and `keydb-cli` parity. Commands come only from code built requests (SCAN, TYPE, TTL, GET, HGETALL, and so on), never free text, in v1.

MySQL and MariaDB: `mysql --xml --unbuffered --batch` reading stdin. Per request: `START TRANSACTION READ ONLY; <sql>; ROLLBACK; SELECT 'LR1:<id>:DONE' AS lr;`. The SQL must pass `guard.Check` (no backslash, lexically complete: no open quote, comment or delimiter change). Session start records `CONNECTION_ID()`.

Postgres: `psql -X -q --csv -v ON_ERROR_STOP=0` reading stdin, with the same `PGOPTIONS` as today. Per request: `BEGIN READ ONLY; DECLARE ...; FETCH ...; ROLLBACK;` then `SELECT 'LR1:<id>:DONE';`. Same guard requirement; the backslash ban already blocks psql meta commands. `ON_ERROR_STOP=0` so an error does not end the process; the error text is captured and a `ROLLBACK` always follows. Session start records `pg_backend_pid()`. NULL sentinel is random per session and rotated if a value equal to it is ever seen.

### Isolation between requests

- Mongo: no shell state survives a request (see loader). `maxTimeMS` and `comment: "lr:<id>"` on every operation, cursors closed in `finally`, no sessions or transactions opened.
- SQL: every request is wrapped in its own read only transaction followed by `ROLLBACK`. After `ROLLBACK` the session runs a cheap state check before reuse: Postgres `SELECT current_user, current_setting('transaction_read_only'), pg_current_xact_id_if_assigned() IS NULL`, plus `DISCARD ALL` outside a transaction every request (drops prepared statements, temp tables, `SET` values, cursors, advisory locks; it re-applies role defaults). MySQL: `ROLLBACK`, then check `@@session.transaction_read_only` and `CURRENT_USER()`; temp tables and user variables are cleared by `RESET CONNECTION` where the client supports it (VERIFY per client version), otherwise the guard must deny `SET @`, `CREATE TEMPORARY` and `PREPARE` for session routed reads.
- Redis: no `SELECT`, `MULTI`, `CLIENT`, `SUBSCRIBE` or `MONITOR` from session requests; the session runs `SELECT <db>` itself per request if a logical db is chosen.
- A session serves many callers only when they resolve to the same login. With `lr_ro` that is acceptable because the login itself is the boundary (ADR 082); per user brokered sessions are never shared.

### Timeouts and cancellation

- Request deadline: `database_viewer.query_timeout` (existing) plus a margin. Server side cancel always set: Mongo `maxTimeMS`, Postgres `statement_timeout` (already in `PGOPTIONS`), MySQL `MAX_EXECUTION_TIME` (SELECT only), MariaDB `max_statement_time`.
- On deadline without `DONE`: send a server side cancel through a one shot exec (Mongo `killOp` on ops whose comment is `lr:<id>`, Postgres `pg_cancel_backend(<pid>)`, MySQL `KILL QUERY <connection id>`), then kill the session.
- Killing a session: the Docker API has no exec kill. Close the hijacked connection (EOF on stdin, the loader exits), then, if the exec is still running after a grace period (`ExecInspect`), run `kill -9 <pid>` with a one shot exec using the pid from the ready frame. Count `kill_forced`.
- Start timeout: a session that is not `READY` in `start_timeout` is killed and that key goes to fallback with a cooldown.

### Lifecycle and limits (all live runtime settings, seeded from env)

| Key | Env | Default | Meaning |
| --- | --- | --- | --- |
| `database_viewer.sessions.enabled` | `APP_DB_SESSIONS_ENABLED` | `false` | master switch |
| `database_viewer.sessions.engines` | `APP_DB_SESSIONS_ENGINES` | `mongodb` | comma list of engines allowed to use sessions |
| `database_viewer.sessions.idle_timeout` | `APP_DB_SESSION_IDLE_TIMEOUT` | `5m` | close after this long unused |
| `database_viewer.sessions.max_lifetime` | `APP_DB_SESSION_MAX_LIFETIME` | `30m` | recycle regardless of use |
| `database_viewer.sessions.max_requests` | `APP_DB_SESSION_MAX_REQUESTS` | `500` | recycle after this many requests |
| `database_viewer.sessions.start_timeout` | `APP_DB_SESSION_START_TIMEOUT` | `10s` | ready deadline |
| `database_viewer.sessions.pool_per_key` | `APP_DB_SESSION_POOL_PER_KEY` | `1` | sessions per key (max 4) |
| `database_viewer.sessions.max_per_database` | `APP_DB_SESSION_MAX_PER_DATABASE` | `3` | across logins |
| `database_viewer.sessions.max_per_host` | `APP_DB_SESSION_MAX_PER_HOST` | `12` | across all databases on one node |
| `database_viewer.sessions.max_rss_mb` | `APP_DB_SESSION_MAX_RSS_MB` | `200` | loader reports RSS in each `DONE`; above this the session is recycled |
| `database_viewer.sessions.fallback_cooldown` | `APP_DB_SESSION_FALLBACK_COOLDOWN` | `2m` | after a start failure, one shot only for this key |

Defaults are starting points, not measured. A mongosh session is a Node process of tens of MB (VERIFY on mongo 6, 7, 8 images); `max_per_host` times that must fit next to the database's own memory limit, because the session runs inside the database container and counts against its cgroup. That is the real cap: a session can push a tightly limited database container into OOM. Slice 1 refuses to start a session when the container's memory limit minus current usage is below `max_rss_mb` (read from container stats), and falls back.

Turning `enabled` off closes all sessions within one idle sweep. Changing any limit applies to new sessions; existing ones are recycled at their next boundary.

When a request finds its key's sessions all busy it waits up to a short queue time (reuse the `database_viewer.max_concurrent` semantics) and then takes one shot exec instead of blocking.

### Invalidation

- Docker events `die`, `destroy`, `restart`, `kill`, `oom` for a container close all its sessions (`docker.Client.Events`, already used by the trace code).
- Any request on a session for a container whose current ID differs from the key (looked up as today in `internal/api/database_viewer.go`) closes it.
- Database upgrade, restore, version change, password rotation (`internal/dbrotate`), TLS mode change, and broker login drop call `sessions.CloseDatabase(id)` before they act.
- Control plane restart: sessions die with their hijacked connections; nothing is persisted.

### Fallback

Any session failure (start, auth, protocol, timeout, cap) answers the current request through one shot exec, unchanged from today, and marks the key for `fallback_cooldown`. Doctor warns when a database spent more than half of the last hour in fallback, or when a host is at `max_per_host`. A timed out request is not retried on one shot if the server side cancel did not confirm, to avoid running a heavy query twice; the caller gets the timeout error as today.

### Metrics and debug panel

Exported through the existing metrics registry and stored as database metrics (ADR 080 tiers):

- `dbviewer_session_requests_total{engine,result}` with result `hit`, `cold`, `fallback`, `timeout`, `error`
- `dbviewer_sessions{engine,state}` gauge (`starting`, `idle`, `busy`)
- `dbviewer_session_start_seconds{engine}` and `dbviewer_session_request_seconds{engine,path}` histograms, path `session` or `oneshot`
- `dbviewer_session_kills_total{engine,reason}` with reason `idle`, `lifetime`, `requests`, `rss`, `timeout`, `protocol_violation`, `container_gone`, `forced`, `disabled`

Debug panel: a "Explorer sessions" section on the database page for `read:sensitive` callers (engine, login name, age, requests served, last request ms, RSS, state) and a close button for `root`. No request content, no filter text, no secrets in the panel or metrics labels.

## Alternatives rejected

- Native protocol from the control plane through a tunnel (pgx, the Go Mongo driver, a RESP client, over a published port, the Docker network or the agent tunnel). Fastest and cleanest framing, but it breaks the main property: the control plane would need passwords for every managed database, and Postgres local socket trust does not work over TCP at all. It also needs reachability the control plane does not have (macOS VM runtime, remote nodes, private only databases). It stays the right model for external databases, which already have a stored credential. Worth revisiting for brokered MySQL and Mongo logins whose secret already lives in control plane memory (open question 1).
- Relay binary copied into the container (a stdio to unix socket bridge) so the control plane speaks the native protocol over the exec stream. Removes the client process cost and gives exact framing, but modifies the customer's container, breaks on read only root filesystems and distroless images, and is a new binary to sign and ship. Rejected for now.
- mongosh REPL as the session. Rejected above: code per request, shared state, no frame boundary.
- Faster mongosh flags. Measured, no gain.
- Caching explorer results in the control plane. Hides latency on repeat views only, stale data, and cache keys would need the caller's identity. Not a substitute.
- Sessions for writes in v1. A write session holds an identity that can commit; one mistake in isolation leaks write power to the next caller. Writes are rare and one shot cost is acceptable for them.

## Security review checklist (lr-reviewer, every slice marked SEC)

- Script injection through framing: Mongo requests are JSON data dispatched by a constant table; no request field is ever concatenated into code. SQL sessions only accept statements that passed `guard.Check`; backslash banned; lexically complete.
- Request smuggling: one request in flight; next request only after the matching nonce `DONE`; a swallowed marker ends in kill, never in reuse. Tests send an unterminated quote and confirm the next request goes to a new session.
- Response forgery: per request nonce; only line start prefixes count; values are escaped by the encoder (EJSON, CSV, XML) or ended only by the nonce line.
- Session fixation and identity confusion: key includes login; ready frame `user` checked against key; no identity change inside a session; brokered sessions closed before login drop.
- State leakage: fresh scope per Mongo request, `ROLLBACK` plus `DISCARD ALL` or the MySQL checks for SQL, no `use`, no `SELECT <db>` from callers.
- Resource exhaustion: caps per key, database, host; RSS recycle; container memory headroom check; queue time then one shot; start cooldown.
- Secrets in args or env: args are constant code only; secrets only from the container env (as today) or the stdin auth frame; never logged; frames are never logged, only ids and timings.
- Docker exec stream hijack: the attach stream is owned by one goroutine pair per session; stdout and stderr demultiplexed as today; stderr is never parsed as protocol. Anyone who can attach to the daemon already owns the host, so this is about our own code mixing streams, not an outside attacker.
- Cross caller output: a response is delivered only to the waiter registered for that id; a late frame for a cancelled id kills the session rather than reaching the next caller.
- Audit and history unchanged: the explorer still writes the same audit and history rows per request; the transport change does not skip them.

## Failure modes

| Failure | Effect | Handling |
| --- | --- | --- |
| Client killed (OOM, `kill -9`) mid request | stream EOF | request falls back once to one shot, session removed |
| Container restart or replace | events, ID mismatch | sessions closed, next request starts cold |
| Hung query, server ignores timeout | no `DONE` | server side cancel, kill, timeout error |
| Loader bug prints garbage | protocol violation | kill, fallback, metric |
| mongosh version without needed APIs | start fails | cooldown fallback, doctor warning names the version |
| Database memory tight | headroom check fails | one shot only, doctor hint |
| Control plane restart | all sessions gone | cold start, no data loss |
| Docker daemon restart | streams break | as container restart |
| Agent tunnel drops (remote node) | stream break | as client killed, plus remote node cooldown |

## What cannot be verified locally

- Warm latency on real hosts. The target, p50 under 60 ms warm for Mongo find and collections overview, and under 30 ms for a Redis scan step, is an estimate: exec floor and Node boot are removed, the rest is server time plus stream round trip. Not measured.
- Session memory per engine and image version; behaviour under tight container memory limits.
- Long lived exec streams through the agent tunnel on remote nodes (`internal/agent` supports streaming stdin, but not checked for streams that stay open for 30 minutes, keepalives, or idle cuts by proxies).
- mongosh blocking stdin reads and per line flushing across 1.x and 2.x; `redis-cli` per reply flushing over a pipe; `mysql --unbuffered` behaviour on MariaDB clients; `RESET CONNECTION` availability.
- Postgres and MySQL numbers at all: there is no demo container for them yet.
- DocumentDB (open source) and Dragonfly behaviour; both stay on one shot until checked.

## Testing plan

- Fake session runtime: an in memory `Execer` that speaks the frame protocol, with knobs for delay, garbage lines, wrong ids, partial frames, EOF mid frame, slow start, RSS growth. Pool tests: caps, idle, lifetime, max requests, queue then fallback, cooldown.
- Hostile inputs: documents containing `LR1 <id> DONE`, newlines, U+2028, huge values, invalid UTF 8; Redis values with embedded marker text; SQL with unterminated quotes, dollar quotes, `\` (rejected by guard), `SET`, `PREPARE`, temp tables (state check catches or guard rejects); request after a killed one goes to a new process (asserted by pid).
- Identity: `lr_ro` caller never receives a brokered session; brokered session closed before login drop; ready frame user mismatch kills.
- Chaos against real containers (local Docker, one per engine, tagged and skipped in `-short`): `kill -9` the client inside the container during a request; `docker restart`; `docker rm -f` and recreate with the same name; slow query beyond timeout (`$where` sleep, `pg_sleep`, `SLEEP()`, `DEBUG SLEEP`) confirms server side cancel; memory limited container confirms headroom refusal.
- Latency: the same 20 call method as the baseline, sessions off versus on, cold and warm.
- Setting flips: enable, disable, lower caps while sessions are busy.

## Build slices

SEC means Sonnet plus an lr-reviewer pass.

| # | Slice | Files (owner) | Settings | Review |
| --- | --- | --- | --- | --- |
| 1 | Session core: key, pool, lifecycle, frame codec, waiter by id, caps, cooldown, headroom check, fake runtime and pool tests | `internal/dbsession/*` (new) | all `database_viewer.sessions.*` in `internal/runtimesettings/registry_dbsession.go` (new) | SEC |
| 2 | Long lived exec: `ExecSession` returning writer, reader, exec ID; inspect and forced kill; agent path parity | `internal/docker/client.go`, `internal/docker/runtime.go`, `internal/agent/exec*.go` | none | SEC |
| 3 | Mongo loader spike then loader: embedded constant program, op table, auth frame, ready frame, RSS in `DONE` | `internal/dbviewer/mongo_session.go` (new), `internal/dbviewer/mongo_loader.js` (new, embedded) | none | SEC |
| 4 | Wire Mongo explorer routes through sessions with one shot fallback; audit and history unchanged | `internal/api/database_viewer_mongo.go`, `internal/api/database_viewer.go` | `APP_DB_SESSIONS_ENABLED`, `APP_DB_SESSIONS_ENGINES` | SEC |
| 5 | Invalidation: Docker events, ID mismatch, hooks from upgrade, restore, rotate, TLS and broker sweeper | `internal/dbsession/invalidate.go` (new), `internal/dbrotate/driver.go` (hook only), `internal/dbaccess/sweeper.go` (hook only, sequenced with ADR 082 slice 5) | none | SEC |
| 6 | Metrics, database metric storage, doctor checks | `internal/dbsession/metrics.go` (new), doctor check file next to the existing doctor checks | none | no |
| 7 | Debug panel and close route (`root`) | `internal/api/database_sessions.go` (new), `web/src/components/DatabaseSessionsPanel.tsx` (new), locale keys | none | SEC (close route only) |
| 8 | Redis family session | `internal/dbviewer/redis_session.go` (new), `internal/dbviewer/redis.go` | engines list | SEC |
| 9 | MySQL and MariaDB session with state checks and `KILL QUERY` cancel | `internal/dbviewer/sql_session.go` (new), `internal/dbviewer/run.go`, `internal/dbviewer/guard.go` (session deny list) | engines list | SEC |
| 10 | Postgres session with `DISCARD ALL`, backend pid cancel, chaos suite for all engines | `internal/dbviewer/sql_session.go`, `internal/dbviewer/run.go`, `internal/dbsession/chaos_test.go` (new) | engines list | SEC |

Order: 1 and 2 in parallel, 3 after 2 (needs a live stream for the spike), 4 after 1 and 3, 5 after 4, 6 and 7 after 4 in parallel, 8 after 4, 9 then 10 in sequence (shared `sql_session.go` and `run.go`). Slice 3 is a go or no go gate: if the blocking read loop does not work in mongosh, stop and revise this ADR before slice 4.

Rollout: 1.5.11 ships slices 1 to 6 with `enabled=false`; the owner turns it on for the demo host and vps2 only. The next release turns the default on for `mongodb` after a clean soak (no protocol violations, fallback rate under 5 percent, no OOM attributable to sessions), then adds `redis`, then SQL engines one release each.

## Consequences

- Mongo explorer latency drops from about 650 ms to an expected tens of milliseconds warm; first request after idle stays at today's cost.
- The database container now runs a client process between requests, counted against its memory limit. Users will see an extra process in `docker top`; the panel explains it.
- Mongo explorer requests stop being generated scripts and become data for a fixed program, which shrinks the injection surface even for the one shot path if the loader is reused there later.
- More moving parts: a pool, a protocol, invalidation hooks in upgrade, restore, rotation and the broker sweeper. Each future operation that changes a database's identity or process must call `CloseDatabase`.
- Writes keep one shot latency.

## Open questions

1. For brokered MySQL and Mongo logins, whose secret ADR 082 already keeps in control plane memory, should the explorer use a native driver over the network instead of a session, where reachability exists? This would split the transport by login type.
2. Amend ADR 082 so brokered secrets reach the client by stdin frame instead of exec env, for both one shot and session paths?
3. Should `lr_ro` sessions be shared across all callers of a database (fast, relies on the login as the boundary) or per Levelrail user even for `lr_ro` (more processes, stronger per user attribution via `appName` and `application_name`)? This ADR picks shared; per user attribution then comes from the audit rows, not the engine.
4. Are the cap defaults acceptable before measuring RSS per image, and should a database below a memory limit threshold opt out automatically?
5. Is a 30 minute stream through the agent tunnel acceptable for remote nodes, or should remote nodes stay on one shot until the tunnel has stream keepalives?
6. Should the loader later replace the one shot Mongo script too (one code path, run with EOF after one request), retiring `mongoScript`?
