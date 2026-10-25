# Secrets checklist for stack shop

Source: config files (heroku). This file lists secret names only. No value was read into these files, and none belongs in this repository.

Supply each value at apply time from your CI secret store, for example:

```bash
levelrail-cli stack apply shop -f . --yes --secret clock/CACHE_URL=env:MY_CI_SECRET
```

Use `NAME` to set a value for every app that references it, or `app/NAME` for one app.

## App clock

- [ ] `CACHE_URL`: connection string of redis database cache, set it once the database exists or run databases link
- [ ] `DATABASE_URL`: connection string of postgres database main, set it once the database exists or run databases link
- [ ] `SECRET_KEY_BASE`: the source generated this value: create a random one, for example openssl rand -hex 32
- [ ] `STRIPE_KEY`: required secret, set it before the first deploy

## App web

- [ ] `CACHE_URL`: connection string of redis database cache, set it once the database exists or run databases link
- [ ] `DATABASE_URL`: connection string of postgres database main, set it once the database exists or run databases link
- [ ] `SECRET_KEY_BASE`: the source generated this value: create a random one, for example openssl rand -hex 32
- [ ] `STRIPE_KEY`: required secret, set it before the first deploy

## App worker

- [ ] `CACHE_URL`: connection string of redis database cache, set it once the database exists or run databases link
- [ ] `DATABASE_URL`: connection string of postgres database main, set it once the database exists or run databases link
- [ ] `SECRET_KEY_BASE`: the source generated this value: create a random one, for example openssl rand -hex 32
- [ ] `STRIPE_KEY`: required secret, set it before the first deploy

## Database connections

The documents cannot yet hand a database's connection string to an app (`databases:` sections and `from: postgres.main.url` references arrive with a later schema change). Until then each connection variable above is a secret name placeholder. Create the databases from `databases.yaml`, read each connection string, and supply it as the secret named above, or run `levelrail-cli databases link NAME APP` so the platform injects it.

- `cache` (redis)
- `main` (postgres)
