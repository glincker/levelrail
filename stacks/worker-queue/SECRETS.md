# Secrets

Names only. Never commit a value. Pass each one at apply time with `--secret NAME=env:VAR` from your CI secret store.

- `API_TOKEN`: the token the app and worker use to call your upstream API.
