# Levelrail Python SDK

A small client for the Levelrail control plane API. Standard library only, Python 3.9 or newer.
The package name `levelrail-sdk` is a placeholder and the package is not published. Licensing is pending.

```python
from levelrail import Client

c = Client("https://control.example.com", token)
for app in c.all_apps(limit=50):
    print(app["name"])
c.set_env("web", {"MODE": "prod"})
for ev in c.changes():
    print(ev)
```

The operation table in `levelrail/_operations.py` is generated from `docs/public/openapi.json`
(`go run ./scripts/gen-python-sdk`). Any operation is reachable with `Client.invoke(operation_id, ...)`.
Tokens and environment values never appear in reprs or exceptions.

Tests: `python3 -m unittest discover -s sdk/python/tests` from the repository root.
