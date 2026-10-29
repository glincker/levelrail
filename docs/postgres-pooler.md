# Postgres connection pooler

Small servers often run out of Postgres connections before they run out of CPU or memory. Each app replica opens its own pool, and `max_connections` fills up fast. A managed Postgres database can opt into a pooler: a pgbouncer style container that runs next to the database in transaction pooling mode, so many client connections share a few server connections.

## Enable it

The control plane needs to know which pooler image to run. There is no built in default, so you pick and pin a version you trust:

| Variable | Meaning |
| --- | --- |
| `APP_PG_POOLER_IMAGE` | Required. Pooler image, pinned by tag or digest. It must accept the common pgbouncer env contract (`DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `LISTEN_PORT`, `POOL_MODE`, `AUTH_TYPE`). |
| `APP_PG_POOLER_DEFAULT_POOL_SIZE` | Optional. Server connections per user and database pair. Unset or invalid keeps the image default. |
| `APP_PG_POOLER_MAX_CLIENT_CONN` | Optional. Maximum client connections the pooler accepts. Unset or invalid keeps the image default. |

The two sizing variables are control plane defaults. A single database can override them with `--pool-size` and `--max-client-conn` (CLI), the sizing inputs on the pooler card, or `pooler.pool_size` and `pooler.max_client_conn` in a resource file. `0` or unset keeps the default. Changing them replaces the pooler container.

Then turn it on per database:

```sh
levelrail-cli databases pooler enable main
levelrail-cli databases pooler enable main --pool-size 20 --max-client-conn 400
levelrail-cli databases pooler show main --json
levelrail-cli databases pooler disable main
```

The API is `GET`, `POST` and `DELETE` on `/api/v1/databases/{name}/pooler`. Reading needs the read ability; toggling needs the sensitive write ability, the same as public access, since it changes the URL every linked app receives.

## What changes for apps

The pooler joins the database container's network namespace and listens on port 6432. Apps keep the same host, only the port changes.

- `{ from: { database: main, field: url } }` resolves to the pooled URL while the pooler is on.
- `field: port` resolves to 6432 while the pooler is on.
- `field: direct_url` always resolves to the direct Postgres URL. Use it for migrations, `LISTEN`/`NOTIFY`, advisory locks held across transactions, and anything else that needs a session.

Apps on a different node than the database keep the direct URL, because the mesh only maps the Postgres port.

Transaction pooling means session state does not survive between transactions. Prepared statements, `SET` without `LOCAL`, and temporary tables can misbehave on the pooled URL. Most ORMs have a setting for this (for example disabling server side prepared statements).

## How it is reconciled

The reconciler treats the pooler as a child of the database and converges it on every pass:

- Enabled and the database is running: the pooler container exists and runs. It is recreated when the database container is replaced (a version change), the image changes, or the pool settings change.
- Disabled, suspended or deleted: the pooler container is removed.
- Its health shows up as a separate `PoolerReady` condition, so a broken pooler never marks the database itself as down. `pooler show` prints the reason, for example `PoolerImageNotConfigured`.

The database password is passed to the pooler container the same way it is passed to Postgres itself. It is never written into labels, logs or API responses: the URLs returned by `pooler show` use a `<password>` placeholder.
