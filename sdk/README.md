# Levelrail SDKs

Small clients for the Levelrail control plane API. Each SDK covers the same core: list apps, paginate, and stream change events over SSE. Every other operation in `docs/public/openapi.json` is reachable by its operation id.

| SDK | Path | Dependencies |
| --- | --- | --- |
| Go | `sdk/go/levelrail` | standard library |
| Python | `sdk/python` | standard library, Python 3.9 or newer |
| TypeScript | `sdk/typescript` | none, built-in `fetch` (see its [README](typescript/README.md)) |

The SDKs are not published yet. Install them from this repository.

## Install

Go (the package lives in the root module):

```sh
go get github.com/GLINCKER/levelrail/sdk/go/levelrail
```

Python, from a checkout:

```sh
pip install -e ./sdk/python
```

## Auth with a scoped token

Every call sends `Authorization: Bearer <token>`. Create a token with only the abilities your code needs. Ability `read` covers listing apps, deploy history and the change stream.

```sh
levelrail-cli tokens create --name ci-reader --abilities read
```

Token creation and revocation are session-only on the server. An API token cannot mint another token, so create tokens from an interactive session and store the value in your secret manager. Read it from an environment variable in code:

```sh
export LEVELRAIL_TOKEN="<paste the scoped token>"
```

Never commit the token or print it in logs. The Go and Python clients do not include it in their error messages or `repr`.

## List apps

Go:

```go
client := levelrail.New("https://levelrail.example.com", os.Getenv("LEVELRAIL_TOKEN"))
apps, next, err := client.ListApps(ctx, levelrail.ListOptions{Limit: 50})
```

Python:

```python
client = Client("https://levelrail.example.com", os.environ["LEVELRAIL_TOKEN"])
apps, next_cursor = client.list_apps(limit=50)
```

## Pagination

List routes are cursor paged. The server returns the next cursor in the `X-Next-Cursor` header, and the empty string marks the last page. The SDKs follow the cursor for you.

Go iterates every app with `AllApps`, or with `Paginate` for any list route:

```go
for app, err := range client.AllApps(ctx, levelrail.ListOptions{Limit: 100}) {
	if err != nil {
		return err
	}
	fmt.Println(app["name"])
}
```

Python iterates with `all_apps`:

```python
for app in client.all_apps(limit=100):
    print(app.get("name"))
```

Iteration stops on an empty cursor, on an error, or when you break out of the loop. A repeated cursor also stops it, so a misbehaving server cannot loop forever.

## Stream changes (SSE)

`GET /api/v1/events/changes` sends a text event stream. Each `ping` event is a heartbeat, and every other event is a nudge that names the topics that changed. A nudge tells you to refetch, not what the new state is.

Go:

```go
for ev, err := range client.Changes(ctx) {
	if err != nil {
		// Reconnect and refetch on error or end of stream.
		return err
	}
	if !ev.Heartbeat {
		fmt.Println("changed:", ev.Topics)
	}
}
```

Python:

```python
for event in client.changes():
    if not event.get("heartbeat"):
        print("changed:", event.get("topics"))
```

The stream ends when the server closes it or on error. Reconnect and refetch your state in that case. Cancel the Go context to stop early. The Go client exempts streams from its per-call timeout, and it caps one SSE frame at 4 MiB by default (`WithMaxFrameBytes` changes that).

## Other calls

Go exposes typed helpers such as `GetApp`, `CreateApp`, `DeployApp`, `RollbackApp`, `ListDeploys`, `GetEnv`, `SetEnv` and `Health`. Any other operation runs through `Invoke(ctx, operationID, params, query, body, out)`. Python has matching methods and `invoke(operation_id, ...)`.

Pass an idempotency key to make a create or deploy call safe to retry. Go uses `WithIdempotencyKey(ctx, key)`, and Python uses the `idempotency_key` argument.

## Examples

Runnable Go examples are in [`go/levelrail/example_test.go`](go/levelrail/example_test.go). Python examples are in [`python/examples`](python/examples). They point at `https://levelrail.example.com`, so replace that URL with your control plane before running them.

## Errors

A non-2xx response becomes `*levelrail.APIError` in Go (`Status`, `Message`) and `APIError` in Python. Transport failures are wrapped without the request body or token.
