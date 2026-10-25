# Secrets checklist for stack shop

Source: config files (heroku, vercel). This file lists secret names only. No value was read into these files, and none belongs in this repository.

Supply each value at apply time from your CI secret store, for example:

```bash
levelrail-cli stack apply shop -f . --yes --secret NAME=env:MY_CI_SECRET
```

Use `NAME` to set a value for every app that references it, or `app/NAME` for one app.

No secret names were found.
