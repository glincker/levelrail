# Secrets checklist for stack heroku-import

Source: heroku. This file lists secret names only. No value was read into these files, and none belongs in this repository.

Supply each value at apply time from your CI secret store, for example:

```bash
levelrail-cli stack apply heroku-import -f . --yes --secret shop/SENDGRID_URL=env:MY_CI_SECRET
```

Use `NAME` to set a value for every app that references it, or `app/NAME` for one app.

## App shop

- [ ] `SENDGRID_URL`: secret on the source, the value was not copied
- [ ] `SESSION_SECRET`: secret on the source, the value was not copied

## App shop-worker

- [ ] `SENDGRID_URL`: secret on the source, the value was not copied
- [ ] `SESSION_SECRET`: secret on the source, the value was not copied

## Database connections

Apps read these databases through `from:` references in `apps.yaml`; the platform fills in each value when the container starts, so there is nothing to set here. A connection variable listed above as a secret could not be matched to one database: point it at one with `{from: {database: NAME, field: url}}`, or supply its value.

- `postgresql-round-1` (postgres): shop `DATABASE_URL`, shop-worker `DATABASE_URL`
- `redis-deep-2` (redis): shop `REDIS_URL`, shop-worker `REDIS_URL`
