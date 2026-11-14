# e2e: levelrail_node_labels

Drift then import against a throwaway local instance: apply, change the resource out of band through the API, expect a plan diff, apply again, drop it from state, import it into fresh state and expect an empty plan. See [../README.md](../README.md) for the setup.

```bash
E2E_I_UNDERSTAND_THIS_USES_A_THROWAWAY_INSTANCE=1 \
LEVELRAIL_ENDPOINT=http://127.0.0.1:PORT LEVELRAIL_TOKEN=... \
E2E_NODE=<existing node name> \
./run.sh
```

Out of band change: a label value is rewritten on the node. The node must already exist (the script never creates or deletes nodes), and destroy removes only the label the script set.
