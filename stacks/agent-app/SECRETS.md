# Secrets

Names only. Never commit a value. Pass each one at apply time with `--secret NAME=env:VAR` from your CI secret store.

- `MODEL_API_KEY`: credential for the model provider the agent calls.
- `TOOL_WEBHOOK_SECRET`: verifies incoming tool webhooks.
