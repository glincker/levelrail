# Secrets

Names only. Never commit a value. Pass each one at apply time with `--secret NAME=env:VAR` from your CI secret store.

- `SESSION_SECRET`: signs user sessions. Use a fresh random value per environment.
