# Secrets checklist for stack vercel-import

Source: vercel. This file lists secret names only. No value was read into these files, and none belongs in this repository.

Supply each value at apply time from your CI secret store, for example:

```bash
levelrail-cli stack apply vercel-import -f . --yes --secret web/API_KEY=env:MY_CI_SECRET
```

Use `NAME` to set a value for every app that references it, or `app/NAME` for one app.

## App web

- [ ] `API_KEY`: secret on the source, the value was not copied; the source would not reveal its value
- [ ] `DB_PASS`: secret on the source, the value was not copied; the source would not reveal its value
