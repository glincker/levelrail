import { useCallback, useEffect, useRef, useState } from 'react'

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
//   - a JSON object: { "line": string, "stream": "stdout" | "stderr" }
//
// Both real backends send the JSON shape (sseLogEvent in
// internal/api/deploy_attempts.go), since distinguishing stdout/stderr is
// useful for highlighting failed build steps or a crashing app alike.
// `parseEventPayload` below tries JSON.parse first and falls back to
// treating the whole payload as a bare line, so this hook does not
// hard-fail against either shape. Named SSE events are not used (no
// `event:` field expected), everything arrives as the default `message`
// event, since that is the simplest contract to hand-implement
// server-side with Go's net/http.
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
  stream: 'stdout' | 'stderr'
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
}

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
  stream: 'stdout' | 'stderr'
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

function parseEventPayload(raw: string): ParsedLogEvent {
  try {
    const parsed: unknown = JSON.parse(raw)
    if (isRawLogEventPayload(parsed)) {
      return {
        line: parsed.line,
        stream: parsed.stream === 'stderr' ? 'stderr' : 'stdout',
      }
    }
  } catch {
    // Not JSON: fall through and treat the whole payload as a bare line.
  }
  return { line: raw, stream: 'stdout' }
}

// Ring buffer implementation choice: a plain array in React state, capped
// at MAX_BUFFERED_LINES on every append via slice, rather than a
// ref-backed circular buffer with head/tail pointers. Justification:
// TanStack Virtual (the consumer, see the logs routes) needs stable
// by-index access into a plain array for its `count`/`getVirtualItems`
// API, so a real circular buffer would need to be flattened into an array
// on every render anyway to hand to the virtualizer, which is the same
// cost as just capping a plain array directly. At the throughput a build
// or app log realistically produces (dozens to low hundreds of lines/
// second, not a per-frame stream), an O(n) slice at an 8,000-line cap
// costs low-single-digit microseconds, not a perf concern worth a more
// complex structure for. State (not a ref) because the route needs each
// new line to trigger a re-render for the virtualized list anyway.
export function useLogStream(url: string): UseLogStreamResult {
  const [lines, setLines] = useState<LogLine[]>([])
  const [connectionState, setConnectionState] =
    useState<LogStreamConnectionState>('connecting')
  const [isPaused, setIsPaused] = useState(false)
  const nextIdRef = useRef(0)
  // Tracks the URL the state above was reset for, so a URL change (a
  // navigation to a different app/deploy) can reset the buffer and
  // connection status. This is React's documented "adjusting state when a
  // prop changes" pattern: setState calls during render, not inside the
  // effect body. Resetting from inside the effect below would fire after
  // the new EventSource's first message could already arrive, and the
  // react-hooks/set-state-in-effect lint rule flags synchronous setState
  // calls in an effect body for exactly this cascading-render reason.
  const [urlForReset, setUrlForReset] = useState(url)
  if (url !== urlForReset) {
    setUrlForReset(url)
    setLines([])
    setConnectionState('connecting')
    setIsPaused(false)
  }

  useEffect(() => {
    // Refs may not be mutated during render (only in effects or event
    // handlers), so the id counter resets here rather than alongside the
    // state reset above. This still runs before the new EventSource can
    // deliver its first message, since effects commit after render.
    nextIdRef.current = 0
    let bufferedLines: LogLine[] = []

    // Reconnect-duplicate suppression (see this module's own doc comment
    // above for why the server has no Last-Event-ID resume to rely on
    // instead). hasConnectedOnce distinguishes the very first open
    // (backfill is wanted and correct) from every later one (a same-URL
    // EventSource reconnect after a network drop, whose backfill/replay
    // burst duplicates content already rendered): from the second onopen
    // forward, overlapExpected snapshots everything currently buffered,
    // and each incoming line is compared against it in order. An exact
    // match means "already rendered before the reconnect," dropped
    // silently; the first mismatch (or running out of overlap to compare
    // against, if the resumed burst is longer than what's buffered)
    // clears overlapExpected and every line from there on is appended
    // normally, the same as before this existed.
    let hasConnectedOnce = false
    let overlapExpected: ParsedLogEvent[] = []
    let overlapIndex = 0

    const source = new EventSource(url)

    source.onopen = () => {
      setConnectionState('open')
      if (hasConnectedOnce) {
        overlapExpected = bufferedLines.map((l) => ({ line: l.line, stream: l.stream }))
        overlapIndex = 0
      }
      hasConnectedOnce = true
    }

    // EventSource fires onerror both for a genuine failure and for the
    // moment it drops connection right before it auto-reconnects; either
    // way this only updates displayed status, onopen flips it back once
    // reconnected. No manual retry logic per the module comment above.
    source.onerror = () => {
      setConnectionState('error')
    }

    source.onmessage = (event: MessageEvent<string>) => {
      const parsed = parseEventPayload(event.data)

      const expected = overlapExpected[overlapIndex]
      if (expected !== undefined) {
        if (expected.line === parsed.line && expected.stream === parsed.stream) {
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
        return capped
      })
    }

    return () => {
      source.close()
    }
  }, [url])

  const pause = useCallback(() => {
    setIsPaused(true)
  }, [])

  const resume = useCallback(() => {
    setIsPaused(false)
  }, [])

  return { lines, connectionState, isPaused, pause, resume }
}
