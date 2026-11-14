# e2e: levelrail_setting

Drift then import against a throwaway local instance: apply, change the resource out of band through the API, expect a plan diff, apply again, drop it from state, import it into fresh state and expect an empty plan. See [../README.md](../README.md) for the setup.

```bash
E2E_I_UNDERSTAND_THIS_USES_A_THROWAWAY_INSTANCE=1 \
LEVELRAIL_ENDPOINT=http://127.0.0.1:PORT LEVELRAIL_TOKEN=... \
./run.sh
```

Out of band change: the setting is written directly through the settings API. Destroy at the end resets it to its default.
