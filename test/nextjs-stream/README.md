# Streaming fixture

A dependency free Node server that streams five chunks with a gap between
them, to check that Levelrail's ingress does not buffer streamed responses.

1. Run it: `PORT=3000 GAP_MS=1000 node server.mjs`.
2. Deploy it as an app, or point a domain at `127.0.0.1:3000`.
3. Watch the chunks arrive one per second: `curl -N https://your.domain/` and
   `curl -N https://your.domain/events`.

If the output appears all at once after five seconds, something in the route
is buffering. The automated check is `TestStream_Live` in `internal/ingress`.
