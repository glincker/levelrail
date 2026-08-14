import { useCallback, useEffect, useRef, useState } from 'react'

// Assumed SSE contract for live build/deploy logs (frontend-plan.md
// section 2; build log streaming over SSE and live log tailing are both
// part of the platform's build and frontend design).
// internal/api/router.go has no log-streaming endpoint yet as of this
// pass (TASKS.md 1.9 predates the frontend work), so this is written
// against the documented contract rather than a confirmed one:
//
//   GET /api/v1/apps/{name}/deploys/{deployId}/logs
//   Content-Type: text/event-stream
//
// Each event's `data:` payload is EITHER:
//   - a bare string: the raw log line, no framing, OR
//   - a JSON object: { "line": string, "stream": "stdout" | "stderr" }
//
// The JSON shape is the one this hook prefers once the backend exists,
// since distinguishing stdout/stderr is useful for highlighting failed
// build steps (the route colors stderr lines). `parseEventPayload` below
// tries JSON.parse first and falls back to treating the whole payload as
// a bare line, so this hook does not hard-fail against either shape.
// Named SSE events are not used (no `event:` field expected), everything
// arrives as the default `message` event, since that is the simplest
// contract to hand-implement server-side with Go's net/http.
//
// EventSource reconnects on its own after a dropped connection (this
// clean reconnection behavior, plus being simpler through proxies, is
// exactly why SSE was chosen over WebSockets), so this hook does not
// implement any retry/backoff logic itself.

export interface DeployLogLine {
  /** Monotonic id local to this hook instance, stable virtualization key. */
  id: number
  line: string
  stream: 'stdout' | 'stderr'
}

export type LogStreamConnectionState = 'connecting' | 'open' | 'error'

export interface UseDeployLogStreamResult {
  /** Bounded window of the most recent lines, oldest first. */
  lines: DeployLogLine[]
  connectionState: LogStreamConnectionState
  /** True once the caller has paused (see pause()); ingestion never stops. */
  isPaused: boolean
  /** Marks the stream paused. Purely a flag for the caller (e.g. to stop
   *  auto-scrolling); this hook keeps appending lines regardless. */
  pause: () => void
  resume: () => void
}

// Ring buffer sizing per frontend-plan.md section 2: "last 5,000-10,000
// lines; full history stays queryable from the backend log store per 4.8,
// this is just the live-tail window."
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
// TanStack Virtual (the consumer, see the logs route) needs stable
// by-index access into a plain array for its `count`/`getVirtualItems`
// API, so a real circular buffer would need to be flattened into an array
// on every render anyway to hand to the virtualizer, which is the same
// cost as just capping a plain array directly. At the throughput a build
// log realistically produces (dozens to low hundreds of lines/second,
// not a per-frame stream), an O(n) slice at an 8,000-line cap costs
// low-single-digit microseconds, not a perf concern worth a more complex
// structure for. State (not a ref) because the route needs each new line
// to trigger a re-render for the virtualized list anyway.
export function useDeployLogStream(url: string): UseDeployLogStreamResult {
  const [lines, setLines] = useState<DeployLogLine[]>([])
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

    const source = new EventSource(url)

    source.onopen = () => {
      setConnectionState('open')
    }

    // EventSource fires onerror both for a genuine failure and for the
    // moment it drops connection right before it auto-reconnects; either
    // way this only updates displayed status, onopen flips it back once
    // reconnected. No manual retry logic per the module comment above.
    source.onerror = () => {
      setConnectionState('error')
    }

    source.onmessage = (event: MessageEvent<string>) => {
      const { line, stream } = parseEventPayload(event.data)
      const id = nextIdRef.current
      nextIdRef.current += 1
      setLines((prev) => {
        const withNewLine = [...prev, { id, line, stream }]
        return withNewLine.length > MAX_BUFFERED_LINES
          ? withNewLine.slice(withNewLine.length - MAX_BUFFERED_LINES)
          : withNewLine
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
