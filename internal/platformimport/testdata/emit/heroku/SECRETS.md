# Secrets checklist for stack heroku-import

Source: heroku. This file lists secret names only. No value was read into these files, and none belongs in this repository.

Supply each value at apply time from your CI secret store, for example:

```bash
levelrail-cli stack apply heroku-import -f . --yes --secret shop/DATABASE_URL=env:MY_CI_SECRET
```

Use `NAME` to set a value for every app that references it, or `app/NAME` for one app.

## App shop

- [ ] `DATABASE_URL`: database connection string, created empty here: set it after the database exists, or run databases link
- [ ] `REDIS_URL`: database connection string, created empty here: set it after the database exists, or run databases link
- [ ] `SENDGRID_URL`: secret on the source, the value was not copied
- [ ] `SESSION_SECRET`: secret on the source, the value was not copied

## App shop-worker

- [ ] `DATABASE_URL`: database connection string, created empty here: set it after the database exists, or run databases link
- [ ] `REDIS_URL`: database connection string, created empty here: set it after the database exists, or run databases link
- [ ] `SENDGRID_URL`: secret on the source, the value was not copied
- [ ] `SESSION_SECRET`: secret on the source, the value was not copied

## Database connections

The documents cannot yet hand a database's connection string to an app (`databases:` sections and `from: postgres.main.url` references arrive with a later schema change). Until then each connection variable above is a secret name placeholder. Create the databases from `databases.yaml`, read each connection string, and supply it as the secret named above, or run `levelrail-cli databases link NAME APP` so the platform injects it.

- `postgresql-round-1` (postgres)
- `redis-deep-2` (redis)
