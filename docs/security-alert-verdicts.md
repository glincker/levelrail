---
description: Verdict, evidence, and dismissal command for each code scanning alert open when this page was written.
---

# Security alert verdicts

Verdict, evidence and dismissal command for each code scanning alert that was open when this page was written. Earlier triage landed in #609 and #717; this page covers what remained.

Verdicts use three labels: **fixed** (a real problem, changed in code), **false positive** (a guard exists that the analyzer cannot model, with the guard and the test that proves it), and **accepted risk** (behavior kept on purpose, with the reason).

| Alert | Rule | Location | Verdict |
| --- | --- | --- | --- |
| 43 | `go/allocation-size-overflow` | `internal/agentinit/diff.go` | Fixed |
| 36 | `go/request-forgery` | `internal/preflight/image.go` | False positive |
| 35 | `go/request-forgery` | `internal/preflight/git.go` | False positive |
| 34 | `go/request-forgery` | `internal/importplan/files.go` | False positive |
| 2 | `go/request-forgery` | `internal/alerting/notify.go` | False positive |
| 32 | `go/clear-text-logging` | `internal/secrets/manager.go` | False positive |
| 20 | `go/cookie-secure-not-set` | `internal/api/secure_request.go` | Accepted risk |

## 43: allocation size overflow in the init diff (fixed)

`Diff` built an LCS table sized `(len(a)+1) * (len(b)+1)` from the line counts of an existing file on disk. A very large file made that product huge. `Diff` now checks both line counts and their product against `maxDiffCells` (4M cells) and, past it, reports a whole-file replacement instead of allocating the table. Test: `TestDiff_OversizedFallsBackToReplace` in `internal/agentinit/diff_test.go`. The alert closes when the fix merges and the analysis reruns.

## 36, 35, 34, 2: request forgery (false positive)

Each of these sends an HTTP request to a URL the operator supplies (an image reference, a git repository URL, an import source, an alert webhook). That is the feature, so the URL is user-controlled by design. What matters is that the client cannot reach an internal address.

The guard is `netguard.NewClient()` (`internal/netguard/netguard.go`). It installs a `net.Dialer.Control` hook that runs after DNS resolution on every connection, redirects included, and refuses loopback, link-local (cloud metadata), private and unspecified addresses, including IPv4-mapped IPv6 forms. It also disables proxies, since a proxy would dial on our behalf. Operators who deliberately target internal hosts can opt in with `APP_NOTIFY_ALLOW_PRIVATE_NETWORKS=true`.

This is a dial-time check, not a URL-string sanitizer, so CodeQL's request-forgery query does not see it as a barrier. A string check on the URL would be weaker (DNS rebinding, redirects), so the code is intentionally left as is.

| Alert | Client construction | Test proving the block |
| --- | --- | --- |
| 36 | `internal/api/preflight.go` builds `RegistryInspector{Client: netguard.NewClient()}` | `TestGuardedClientsRefuseInternalAddresses` (`internal/preflight/netguard_test.go`), image cases |
| 35 | `internal/api/preflight.go` builds `SmartHTTPChecker{Client: netguard.NewClient()}` | same test, git cases |
| 34 | `NewHTTPFiles` in `internal/importplan/files.go` starts from `netguard.NewClient()` | `internal/netguard/netguard_test.go` (`TestNewClient_RefusesLoopbackAndMetadata`) |
| 2 | `NewNotifier` in `internal/alerting/notify.go` defaults a nil client to `netguard.NewClient()` | `TestNewNotifier_DefaultClient_BlocksInternal` (`internal/alerting/notify_test.go`) |

Caveat for 2: a caller that passes its own non-nil client to `NewNotifier` bypasses the default. Production callers pass nil or a guarded client; tests pass plain clients to reach `httptest` servers.

## 32: clear-text logging in the secrets manager (false positive)

The flagged call is `noteLegacy` in `internal/secrets/manager.go`, which logs a debug line when a legacy unbound ciphertext is read. It logs the scope constant and the env var **name** (`envKey`), never the value or any key material. CodeQL matched the identifier text rather than a secret flow. Decrypted plaintext is only returned to the caller and is never passed to a logger in this package.

## 20: session cookie without an unconditional Secure flag (accepted risk)

`setSessionCookie` in `internal/api/secure_request.go` sets `Secure` from `requestIsHTTPS(r)`: true for direct TLS, or for `X-Forwarded-Proto: https` from a loopback peer (the embedded ingress). It is false only on plain HTTP.

That is deliberate. A fresh install is reachable over HTTP until the operator configures an HTTPS dashboard URL, and a `Secure` cookie would make first login impossible in that window. The exposure is limited: `X-Forwarded-Proto` is trusted only from loopback, `HttpOnly` and `SameSite=Lax` are always set, and remote HTTP login is refused once an HTTPS dashboard URL is configured. Tests: `TestRequestIsHTTPS`, `TestSessionCookieSecureFollowsRequest`, `TestInsecureLoginRefusal` in `internal/api/secure_request_test.go`.

## Dismissal commands

Run these by hand after review. Alert 43 needs no command.

```bash
for n in 36 35 34 2; do
  gh api -X PATCH "repos/glincker/levelrail/code-scanning/alerts/$n" \
    -f state=dismissed -f dismissed_reason="false positive" \
    -f dismissed_comment="Every request goes through netguard.NewClient(), which refuses loopback, link-local, private and metadata addresses at dial time, redirects included. See docs/security-alert-verdicts.md."
done

gh api -X PATCH repos/glincker/levelrail/code-scanning/alerts/32 \
  -f state=dismissed -f dismissed_reason="false positive" \
  -f dismissed_comment="The log line carries the env var name only, never a value or key material. See docs/security-alert-verdicts.md."

gh api -X PATCH repos/glincker/levelrail/code-scanning/alerts/20 \
  -f state=dismissed -f dismissed_reason="won't fix" \
  -f dismissed_comment="Secure follows the transport by design so first login works over HTTP before an HTTPS dashboard URL is set. See docs/security-alert-verdicts.md."
```
