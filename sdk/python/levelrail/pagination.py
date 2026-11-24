"""Cursor pagination and server-sent events helpers."""
from __future__ import annotations

import json
from typing import Any, Callable, Dict, Iterable, Iterator, List, Tuple, TypeVar

T = TypeVar("T")


def paginate(fetch: Callable[[str], Tuple[List[T], str]]) -> Iterator[T]:
    """Walk a cursor-paged list. fetch(cursor) returns (items, next_cursor)."""
    cursor = ""
    while True:
        items, nxt = fetch(cursor)
        for item in items:
            yield item
        # A repeated cursor would loop forever on a misbehaving server.
        if not nxt or nxt == cursor:
            return
        cursor = nxt


def read_sse(lines: Iterable[Any]) -> Iterator[Dict[str, str]]:
    """Parse text/event-stream lines (str or bytes) into event, id and data dicts."""
    event = ""
    ident = ""
    data: List[str] = []
    for raw in lines:
        line = raw.decode("utf-8", "replace") if isinstance(raw, bytes) else raw
        line = line.rstrip("\r\n")
        if line == "":
            if event or ident or data:
                yield {"event": event, "id": ident, "data": "\n".join(data)}
            event, ident, data = "", "", []
            continue
        if line.startswith(":"):
            continue
        field, _, value = line.partition(":")
        if value.startswith(" "):
            value = value[1:]
        if field == "event":
            event = value
        elif field == "id":
            ident = value
        elif field == "data":
            data.append(value)
    if event or ident or data:
        yield {"event": event, "id": ident, "data": "\n".join(data)}


def changes(client: Any) -> Iterator[Dict[str, Any]]:
    """Stream GET /api/v1/events/changes until the server closes it.

    Yields {"topics": [...]} nudges and {"heartbeat": True, "heartbeat_ms": n}
    pings. Callers reconnect and refetch on error or end of stream.
    """
    resp = client.open_stream("handleChangesStream")
    try:
        for raw in read_sse(resp):
            if raw["event"] == "ping":
                try:
                    ms = int(json.loads(raw["data"]).get("heartbeat_ms", 0))
                except (ValueError, AttributeError):
                    ms = 0
                yield {"heartbeat": True, "heartbeat_ms": ms}
                continue
            try:
                msg = json.loads(raw["data"])
            except ValueError:
                continue
            if isinstance(msg, dict):
                yield {"topics": list(msg.get("topics") or [])}
    finally:
        resp.close()
