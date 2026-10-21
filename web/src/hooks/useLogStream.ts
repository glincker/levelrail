import { useCallback, useEffect, useRef, useState } from 'react'
import type { AppRunStatus } from '../types/containerExit'

// Generic SSE log-tailing hook, shared by two real backends behind the
// same wire contract: GET /api/v1/apps/{name}/deploys/{deployId}/logs
// (one build/deploy attempt's output, internal/api/deploy_attempts.go)
// and GET /api/v1/apps/{name}/logs/stream (a running app container's
// live output, internal/api/live_logs.go). Originally written and named
// for the deploy-log case only (useDeployLogStream/DeployLogLine); when
// the live app-log endpoint landed with the identical contract, this
// hook was generalized in place rather than duplicated, since nothing
// about its connection handling, ring buffer, or pause/resume logic was
// ever actually deploy-specific, only its name and doc comments were.
// Both backends hold the connection open indefinitely rather than
// closing it once they run out of data to send right now (see either
// handler's own doc comment for why), so this hook's lack of manual
// reconnect logic is safe for both: EventSource only ever needs to
// reconnect on a genuine network drop, not as part of normal operation.
//
//   Content-Type: text/event-stream
//
// Each event's `data:` payload is EITHER:
//   - a bare string: the raw log line, no framing, OR
//   - a JSON object: { "line": string, "stream": "stdout" | "stderr" | "system" }
//     ("system" is the live app-log stream's own container-cutover
//     marker, see LogLine.stream's own doc comment.)
//
// Both real backends send the JSON shape (sseLogEvent in
// internal/api/deploy_attempts.go), since distinguishing stdout/stderr is
// useful for highlighting failed build steps or a crashing app alike.
// `parseEventPayload` below tries JSON.parse first and falls back to
// treating the whole payload as a bare line, so this hook does not
// hard-fail against either shape. Log lines arrive as the default
// `message` event; the live app-log stream also sends a named `status`
// event (running, or how the container exited), exposed as runStatus.
//
// EventSource reconnects on its own after a dropped connection (this
// clean reconnection behavior, plus being simpler through proxies, is
// exactly why SSE was chosen over WebSockets), so this hook does not
// implement any retry/backoff logic itself.
//
// Neither backend supports SSE's own Last-Event-ID resume (no `id:`
// field is ever sent, see either handler's own doc comment on its
// backfill/replay burst), so a same-URL reconnect after a genuine
// network drop re-runs that same burst from scratch: up to the last 200
// lines for a live app/database log, or the whole persisted log for a
// deploy log. Left uncorrected, every such reconnect would visibly
// duplicate onto whatever this hook already rendered, since onmessage
// only ever appends. See suppressReconnectDuplicates below for the
// client-side fix.

export interface LogLine {
  /** Monotonic id local to this hook instance, stable virtualization key. */
  id: number
  line: string
  // 'system' is a control-plane-synthesized marker, not real container
  // output (internal/api/live_logs.go's previousContainerTransitionLine):
  // a live app log stream cutting from one container to another mid-session
  // after a deploy. Only the live app-log backend ever sends it; the
  // deploy-log backend never does.
  stream: 'stdout' | 'stderr' | 'system'
}

export type LogStreamConnectionState = 'connecting' | 'open' | 'error'

export interface UseLogStreamResult {
  /** Bounded window of the most recent lines, oldest first. */
  lines: LogLine[]
  connectionState: LogStreamConnectionState
  /** True once the caller has paused (see pause()); ingestion never stops. */
  isPaused: boolean
  /** Marks the stream paused. Purely a flag for the caller (e.g. to stop
   *  auto-scrolling); this hook keeps appending lines regardless. */
  pause: () => void
  resume: () => void
  /** Latest `event: status` from the live app-log stream: null until one
   *  arrives, and always null for the deploy-log stream, which sends none. */
  runStatus: AppRunStatus | null
  /** How lines are arriving: SSE, the polling fallback, or intentionally closed. */
  transport: LogStreamTransport
  /** Epoch ms of the last open or message event, null before the first. */
  lastEventAt: number | null
  /** Epoch ms of the next scheduled reconnect attempt, null when none. */
  retryAt: number | null
  /** Reconnect now (or leave the polling fallback and try SSE again). */
  retry: () => void
  /** Stop retrying SSE and refresh through the plain-text endpoint. */
  switchToPolling: () => void
}

export type LogStreamTransport = 'sse' | 'polling' | 'closed'

export interface UseLogStreamOptions {
  /** True once the stream is no longer wanted (the deploy is terminal). */
  closed?: boolean
  /** Plain-text endpoint to poll when SSE cannot be established. */
  pollUrl?: string
}

// Failed opens before the proxy is assumed to be buffering or blocking SSE.
const POLL_FALLBACK_AFTER_FAILURES = 3
const POLL_INTERVAL_MS = 3000
const MAX_RETRY_DELAY_MS = 15000

// Ring buffer sized to the last 5,000-10,000 lines; full history stays
// queryable from the backend log store per 4.8, this is just the
// live-tail window. Shared by both consumers: a
// long-running app container can realistically produce far more total
// output over its lifetime than a single build ever does, but the same
// cap is still the right call for a *live-tail* window specifically
// (older lines belong to the persisted, searchable store, not this
// in-memory buffer, for either consumer).
const MAX_BUFFERED_LINES = 8000

interface ParsedLogEvent {
  line: string
  stream: 'stdout' | 'stderr' | 'system'
}

interface RawLogEventPayload {
  line: string
  stream?: unknown
}

function isRawLogEventPayload(value: unknown): value is RawLogEventPayload {
  return (
    typeof value === 'object' &&
    value !== null &&
    'line' in value &&
    typeof value.line === 'string'
  )
}

export function parseRunStatus(raw: string): AppRunStatus | null {
  try {
    const parsed: unknown = JSON.parse(raw)
    if (
      typeof parsed === 'object' &&
      parsed !== null &&
      'running' in parsed &&
      typeof parsed.running === 'boolean'
    ) {
      return parsed as AppRunStatus
    }
  } catch {
    // A malformed status event is ignored rather than breaking the stream.
  }
  return null
}

function parseEventPayload(raw: string): ParsedLogEvent {
  try {
    const parsed: unknown = JSON.parse(raw)
    if (isRawLogEventPayload(parsed)) {
      let stream: ParsedLogEvent['stream'] = 'stdout'
      if (parsed.stream === 'stderr') {
        stream = 'stderr'
      } else if (parsed.stream === 'system') {
        stream = 'system'
      }
      return { line: parsed.line, stream }
    }
  } catch {
    // Not JSON: fall through and treat the whole payload as a bare line.
  }
  return { line: raw, stream: 'stdout' }
}

export function useLogStream(
  url: string,
  options: UseLogStreamOptions = {},
): UseLogStreamResult {
  const { closed = false, pollUrl } = options
  const [lines, setLines] = useState<LogLine[]>([])
  const [connectionState, setConnectionState] =
    useState<LogStreamConnectionState>('connecting')
  const [isPaused, setIsPaused] = useState(false)
  const [runStatus, setRunStatus] = useState<AppRunStatus | null>(null)
  const [lastEventAt, setLastEventAt] = useState<number | null>(null)
  const [retryAt, setRetryAt] = useState<number | null>(null)
  const [polling, setPolling] = useState(false)
  const [retryNonce, setRetryNonce] = useState(0)
  const nextIdRef = useRef(0)
  const linesRef = useRef<LogLine[]>([])
  const urlRef = useRef(url)
  // Reset during render (React's "adjust state on prop change" pattern), not
  // in the effect, so the new stream never races stale state.
  const [urlForReset, setUrlForReset] = useState(url)
  if (url !== urlForReset) {
    setUrlForReset(url)
    setLines([])
    setConnectionState('connecting')
    setIsPaused(false)
    setRunStatus(null)
    setLastEventAt(null)
    setRetryAt(null)
    setPolling(false)
  }

  useEffect(() => {
    if (closed || polling) return
    if (urlRef.current !== url) {
      urlRef.current = url
      linesRef.current = []
    }
    // Lines already rendered (manual retry) must not be appended twice.
    let bufferedLines: LogLine[] = linesRef.current
    nextIdRef.current = (bufferedLines.at(-1)?.id ?? -1) + 1

    // The server has no Last-Event-ID resume, so a reconnect replays from
    // the start: compare it against what is buffered and drop the overlap.
    let hasConnectedOnce = bufferedLines.length > 0
    let overlapExpected: ParsedLogEvent[] = []
    let overlapIndex = 0
    let failures = 0
    let source: EventSource | null = null
    let timer: number | undefined
    let disposed = false

    const connect = () => {
      const es = new EventSource(url)
      source = es

      es.onopen = () => {
        failures = 0
        setRetryAt(null)
        setConnectionState('open')
        setLastEventAt(Date.now())
        if (hasConnectedOnce) {
          overlapExpected = bufferedLines.map((l) => ({
            line: l.line,
            stream: l.stream,
          }))
          overlapIndex = 0
        }
        hasConnectedOnce = true
      }

      es.onerror = () => {
        if (disposed) return
        setConnectionState('error')
        failures += 1
        if (!hasConnectedOnce && failures >= POLL_FALLBACK_AFTER_FAILURES) {
          es.close()
          setRetryAt(null)
          setPolling(true)
          return
        }
        if (es.readyState === EventSource.CLOSED) {
          const delay = Math.min(
            POLL_INTERVAL_MS * 2 ** (failures - 1),
            MAX_RETRY_DELAY_MS,
          )
          setRetryAt(Date.now() + delay)
          timer = window.setTimeout(() => {
            setRetryAt(null)
            connect()
          }, delay)
        } else {
          setRetryAt(Date.now() + POLL_INTERVAL_MS)
        }
      }

      es.onmessage = (event: MessageEvent<string>) => {
        setLastEventAt(Date.now())
        const parsed = parseEventPayload(event.data)

        const expected = overlapExpected[overlapIndex]
        if (expected !== undefined) {
          if (
            expected.line === parsed.line &&
            expected.stream === parsed.stream
          ) {
            overlapIndex += 1
            return
          }
          overlapExpected = []
        }

        const { line, stream } = parsed
        const id = nextIdRef.current
        nextIdRef.current += 1
        setLines((prev) => {
          const withNewLine = [...prev, { id, line, stream }]
          const capped =
            withNewLine.length > MAX_BUFFERED_LINES
              ? withNewLine.slice(withNewLine.length - MAX_BUFFERED_LINES)
              : withNewLine
          bufferedLines = capped
          linesRef.current = capped
          return capped
        })
      }

      es.addEventListener('status', (event: MessageEvent<string>) => {
        const status = parseRunStatus(event.data)
        if (status) setRunStatus(status)
      })
    }

    connect()

    return () => {
      disposed = true
      window.clearTimeout(timer)
      source?.close()
    }
  }, [url, closed, polling, retryNonce])

  useEffect(() => {
    if (!polling || !pollUrl) return
    let cancelled = false
    const tick = async () => {
      try {
        const res = await fetch(pollUrl, { credentials: 'same-origin' })
        if (!res.ok || cancelled) return
        const text = await res.text()
        if (cancelled) return
        const next = text
          .split('\n')
          .filter((l) => l.length > 0)
          .slice(-MAX_BUFFERED_LINES)
          .map((line, id) => ({ id, line, stream: 'stdout' as const }))
        linesRef.current = next
        nextIdRef.current = next.length
        setLines(next)
        setLastEventAt(Date.now())
      } catch {
        // The next tick retries; the badge keeps showing the last event time.
      }
    }
    void tick()
    if (closed) return
    const id = window.setInterval(() => void tick(), POLL_INTERVAL_MS)
    return () => {
      cancelled = true
      window.clearInterval(id)
    }
  }, [polling, pollUrl, closed])

  const pause = useCallback(() => {
    setIsPaused(true)
  }, [])

  const resume = useCallback(() => {
    setIsPaused(false)
  }, [])

  const retry = useCallback(() => {
    setPolling(false)
    setRetryAt(null)
    setConnectionState('connecting')
    setRetryNonce((n) => n + 1)
  }, [])

  const switchToPolling = useCallback(() => {
    setRetryAt(null)
    setPolling(true)
  }, [])

  const transport: LogStreamTransport = closed
    ? 'closed'
    : polling
      ? 'polling'
      : 'sse'

  return {
    lines,
    connectionState,
    isPaused,
    pause,
    resume,
    runStatus,
    transport,
    lastEventAt,
    retryAt,
    retry,
    switchToPolling,
  }
}
