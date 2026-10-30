# Secrets checklist for stack shop

Source: config files (heroku). This file lists secret names only. No value was read into these files, and none belongs in this repository.

Supply each value at apply time from your CI secret store, for example:

```bash
levelrail-cli stack apply shop -f . --yes --secret clock/SECRET_KEY_BASE=env:MY_CI_SECRET
```

Use `NAME` to set a value for every app that references it, or `app/NAME` for one app.

## App clock

- [ ] `SECRET_KEY_BASE`: the source generated this value: create a random one, for example openssl rand -hex 32
- [ ] `STRIPE_KEY`: required secret, set it before the first deploy

## App web

- [ ] `SECRET_KEY_BASE`: the source generated this value: create a random one, for example openssl rand -hex 32
- [ ] `STRIPE_KEY`: required secret, set it before the first deploy

## App worker

- [ ] `SECRET_KEY_BASE`: the source generated this value: create a random one, for example openssl rand -hex 32
- [ ] `STRIPE_KEY`: required secret, set it before the first deploy

## Database connections

Apps read these databases through `from:` references in `apps.yaml`; the platform fills in each value when the container starts, so there is nothing to set here. A connection variable listed above as a secret could not be matched to one database: point it at one with `{from: {database: NAME, field: url}}`, or supply its value.

- `cache` (redis): clock `CACHE_URL`, web `CACHE_URL`, worker `CACHE_URL`
- `main` (postgres): clock `DATABASE_URL`, web `DATABASE_URL`, worker `DATABASE_URL`
