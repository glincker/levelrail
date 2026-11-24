"""Hand-written client core. Standard library only, Python 3.9 compatible."""
from __future__ import annotations

import json
import urllib.error
import urllib.parse
import urllib.request
from typing import Any, Dict, Iterator, List, Mapping, Optional, Sequence, Tuple

from ._operations import OPERATIONS

NEXT_CURSOR_HEADER = "X-Next-Cursor"

Object = Dict[str, Any]


class LevelrailError(Exception):
    """Base class for SDK errors."""


class APIError(LevelrailError):
    """A non-2xx response. The message never echoes request bodies."""

    def __init__(self, status: int, message: str) -> None:
        super().__init__("levelrail: HTTP %d: %s" % (status, message))
        self.status = status
        self.message = message


def build_path(template: str, params: Sequence[str]) -> str:
    """Fill {placeholders} in order with percent-escaped params."""
    out: List[str] = []
    used = 0
    rest = template
    while True:
        start = rest.find("{")
        if start < 0:
            out.append(rest)
            break
        end = rest.find("}", start)
        if end < 0 or used >= len(params):
            raise LevelrailError("levelrail: path %r needs more parameters" % template)
        out.append(rest[:start])
        out.append(urllib.parse.quote(params[used], safe=""))
        used += 1
        rest = rest[end + 1:]
    if used != len(params):
        raise LevelrailError(
            "levelrail: path %r takes %d parameters, got %d" % (template, used, len(params))
        )
    return "".join(out)


class Client:
    """Talks to one control plane with a bearer API token."""

    def __init__(self, base_url: str, token: str, timeout: Optional[float] = 30.0) -> None:
        self._base_url = base_url.rstrip("/")
        self._token = token
        self.timeout = timeout

    def __repr__(self) -> str:
        # The token is never part of the repr.
        return "Client(base_url=%r)" % self._base_url

    def _open(
        self,
        op_id: str,
        params: Sequence[str],
        query: Optional[Mapping[str, str]],
        body: Optional[Any],
        accept: str,
        idempotency_key: Optional[str],
    ) -> Any:
        op = OPERATIONS.get(op_id)
        if op is None:
            raise LevelrailError("levelrail: unknown operation %r" % op_id)
        url = self._base_url + build_path(op.path, params)
        if query:
            url += "?" + urllib.parse.urlencode(query)
        data = None
        headers = {"Authorization": "Bearer " + self._token, "Accept": accept}
        if body is not None:
            data = json.dumps(body).encode("utf-8")
            headers["Content-Type"] = "application/json"
        if idempotency_key:
            headers["Idempotency-Key"] = idempotency_key
        req = urllib.request.Request(url, data=data, headers=headers, method=op.method)
        try:
            return urllib.request.urlopen(req, timeout=self.timeout)  # noqa: S310
        except urllib.error.HTTPError as err:
            raise _api_error(err) from None
        except urllib.error.URLError as err:
            # Only the reason is kept, never the request.
            raise LevelrailError("levelrail: request failed: %s" % (err.reason,)) from None

    def invoke(
        self,
        op_id: str,
        params: Sequence[str] = (),
        query: Optional[Mapping[str, str]] = None,
        body: Optional[Any] = None,
        idempotency_key: Optional[str] = None,
    ) -> Tuple[Any, Mapping[str, str]]:
        """Call any operation by id. Returns the decoded JSON and the response headers."""
        resp = self._open(op_id, params, query, body, "application/json", idempotency_key)
        with resp:
            raw = resp.read()
            headers = dict(resp.headers.items())
        if not raw.strip():
            return None, headers
        try:
            return json.loads(raw), headers
        except ValueError:
            raise LevelrailError("levelrail: decode response failed") from None

    def open_stream(self, op_id: str) -> Any:
        """Open a text/event-stream response. The caller closes it."""
        return self._open(op_id, (), None, None, "text/event-stream", None)

    def list_apps(
        self, limit: int = 0, cursor: str = "", query: str = ""
    ) -> Tuple[List[Object], str]:
        """One page of apps and the next cursor ('' at the end)."""
        q: Dict[str, str] = {}
        if limit > 0:
            q["limit"] = str(limit)
        if cursor:
            q["cursor"] = cursor
        if query:
            q["q"] = query
        data, headers = self.invoke("handleListApps", query=q)
        return data or [], _header(headers, NEXT_CURSOR_HEADER)

    def get_app(self, name: str) -> Object:
        data, _ = self.invoke("handleGetApp", [name])
        return data

    def create_app(self, body: Object, idempotency_key: Optional[str] = None) -> Object:
        data, _ = self.invoke("handleCreateApp", body=body, idempotency_key=idempotency_key)
        return data

    def deploy_app(
        self, name: str, body: Optional[Object] = None, idempotency_key: Optional[str] = None
    ) -> Object:
        data, _ = self.invoke(
            "handleTriggerDeploy", [name], body=body, idempotency_key=idempotency_key
        )
        return data

    def rollback_app(self, name: str, deploy_id: str) -> Object:
        data, _ = self.invoke("handleRollbackToDeploy", [name, deploy_id], body={})
        return data

    def list_deploys(self, name: str) -> List[Object]:
        data, _ = self.invoke("handleDeployHistory", [name])
        return data or []

    def get_env(self, name: str) -> List[Object]:
        """Variables bound to an app. The server masks secret values."""
        data, _ = self.invoke("handleListBoundEnv", [name])
        return data or []

    def set_env(self, name: str, variables: Mapping[str, str]) -> Object:
        """Replace the plain environment of an app. Values are never put into errors."""
        data, _ = self.invoke("handleUpdateApp", [name], body={"env": dict(variables)})
        return data

    def health(self) -> Object:
        data, _ = self.invoke("handleHealthz")
        return data

    def all_apps(self, limit: int = 0, query: str = "") -> Iterator[Object]:
        """Iterate every app, following X-Next-Cursor page by page."""
        from .pagination import paginate

        return paginate(lambda cursor: self.list_apps(limit, cursor, query))

    def changes(self) -> Iterator[Object]:
        """Stream events/changes. See pagination.changes for the event shape."""
        from .pagination import changes

        return changes(self)


def _header(headers: Mapping[str, str], name: str) -> str:
    lower = name.lower()
    for k, v in headers.items():
        if k.lower() == lower:
            return v
    return ""


def _api_error(err: urllib.error.HTTPError) -> APIError:
    try:
        raw = err.read(1 << 16).decode("utf-8", "replace").strip()
    except Exception:  # noqa: BLE001 - body is best effort
        raw = ""
    finally:
        err.close()
    msg = raw
    try:
        parsed = json.loads(raw)
        if isinstance(parsed, dict) and isinstance(parsed.get("error"), str) and parsed["error"]:
            msg = parsed["error"]
    except ValueError:
        pass
    return APIError(err.code, msg or (err.reason if isinstance(err.reason, str) else "") or "error")
