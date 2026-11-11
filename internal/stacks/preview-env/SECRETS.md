# Secrets

Names only. Never commit a value. Pass each one at apply time with `--secret NAME=env:VAR` from your CI secret store.

None. The database URL is injected by the platform. Add a `secretRef` to `app.yaml` if the app needs more.
