# Secrets checklist for stack render-import

Source: render. This file lists secret names only. No value was read into these files, and none belongs in this repository.

Supply each value at apply time from your CI secret store, for example:

```bash
levelrail-cli stack apply render-import -f . --yes --secret api/JWT_SALT=env:MY_CI_SECRET
```

Use `NAME` to set a value for every app that references it, or `app/NAME` for one app.

## App api

- [ ] `JWT_SALT`: secret on the source, the value was not copied; the source would not reveal its value
- [ ] `SESSION_SECRET`: secret on the source, the value was not copied

## Database connections

Apps read these databases through `from:` references in `apps.yaml`; the platform fills in each value when the container starts, so there is nothing to set here. A connection variable listed above as a secret could not be matched to one database: point it at one with `{from: {database: NAME, field: url}}`, or supply its value.

- `cache` (redis)
- `main-db` (postgres)
