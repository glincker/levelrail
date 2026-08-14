import { createFileRoute } from '@tanstack/react-router'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useCallback, useEffect, useRef } from 'react'
import { ArrowDownIcon, TerminalIcon } from '@phosphor-icons/react/dist/ssr'
import {
  useDeployLogStream,
  type LogStreamConnectionState,
} from '../../../../../hooks/useDeployLogStream'
import { buildDeployLogStreamUrl } from '../../../../../queries/deployLogs'
import { stripAnsiCodes } from '../../../../../lib/ansi'
import { Badge } from '@/components/ui/badge'

// Live Build Log view, frontend-plan.md section 1: the literal target of
// the rule that the dashboard should not ship the log viewer's bundle.
// This route file is the only place in the app that imports
// the SSE hook, the ANSI stripper, and (here, directly) TanStack Virtual
// tuned for monospace log rows, so none of the three should ever end up
// in a shared/main chunk. Verified for this pass via `npm run build`'s
// per-chunk output, see the report for this change.
//
// No loader here (contrast web/src/routes/apps/index.tsx): this route has
// no fetched resource to prime into TanStack Query's cache. The log
// stream is a live, append-only feed from useDeployLogStream, which is
// deliberately a separate data layer from Query (frontend-plan.md section
// 2: a live stream does not fit Query's freshness/staleness model).
export const Route = createFileRoute('/apps/$name/deploys/$deployId/logs')({
  component: DeployLogsPage,
})

const ROW_HEIGHT_PX = 20
// How close to the bottom (px) still counts as "at the bottom" for
// auto-scroll purposes. scrollHeight/clientHeight are subject to
// sub-pixel rounding, so an exact-zero comparison would flap.
const BOTTOM_THRESHOLD_PX = 32

function DeployLogsPage() {
  const { name, deployId } = Route.useParams()
  const url = buildDeployLogStreamUrl(name, deployId)
  const { lines, connectionState, isPaused, pause, resume } =
    useDeployLogStream(url)

  const parentRef = useRef<HTMLDivElement>(null)
  // Distinguishes a scroll event caused by the auto-scroll effect below
  // from one caused by the user dragging the scrollbar/wheel/trackpad, so
  // the scroll handler does not immediately re-pause right after an
  // auto-scroll-to-bottom.
  const isAutoScrollingRef = useRef(false)

  const virtualizer = useVirtualizer({
    count: lines.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ROW_HEIGHT_PX,
    overscan: 20,
  })

  // Auto-scroll to the newest line while live and not paused. Pausing
  // (set when the user scrolls up manually, see handleScroll) stops this
  // effect from moving the viewport, but never stops ingestion: lines
  // keep arriving into the ring buffer via the hook regardless of
  // isPaused, exactly per frontend-plan.md section 3's "scrolling up
  // disables auto-scroll without stopping ingestion."
  useEffect(() => {
    if (isPaused || lines.length === 0) {
      return
    }
    isAutoScrollingRef.current = true
    virtualizer.scrollToIndex(lines.length - 1, { align: 'end' })
    const clearGuard = window.setTimeout(() => {
      isAutoScrollingRef.current = false
    }, 0)
    return () => {
      window.clearTimeout(clearGuard)
    }
  }, [lines.length, isPaused, virtualizer])

  const handleScroll = useCallback(() => {
    if (isAutoScrollingRef.current) {
      return
    }
    const el = parentRef.current
    if (!el) {
      return
    }
    const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight
    const atBottom = distanceFromBottom <= BOTTOM_THRESHOLD_PX
    if (atBottom && isPaused) {
      resume()
    } else if (!atBottom && !isPaused) {
      pause()
    }
  }, [isPaused, pause, resume])

  const handleResumeClick = useCallback(() => {
    resume()
    if (lines.length > 0) {
      virtualizer.scrollToIndex(lines.length - 1, { align: 'end' })
    }
  }, [resume, lines.length, virtualizer])

  const virtualItems = virtualizer.getVirtualItems()

  return (
    <div className="flex h-full flex-col gap-3">
      <header className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <h1 className="flex items-center gap-1.5 text-lg font-semibold text-foreground">
            <TerminalIcon
              className="size-4 text-muted-foreground"
              aria-hidden="true"
            />
            Build logs
          </h1>
          <p className="truncate text-xs text-muted-foreground">
            {name} / deploy {deployId}
            {lines.length > 0
              ? ` · ${lines.length.toLocaleString()} lines`
              : ''}
          </p>
        </div>
        <ConnectionBadge state={connectionState} />
      </header>

      <div className="relative flex-1">
        <div
          ref={parentRef}
          onScroll={handleScroll}
          className="h-[70vh] overflow-auto rounded-lg border border-neutral-800 bg-neutral-950 font-mono text-xs leading-5 text-neutral-200"
        >
          {lines.length === 0 ? (
            <p className="flex items-center gap-2 px-3 py-2 text-neutral-500">
              <span className="size-1.5 animate-pulse rounded-full bg-neutral-500" />
              Waiting for log output...
            </p>
          ) : (
            <div
              style={{
                height: virtualizer.getTotalSize(),
                position: 'relative',
              }}
            >
              {virtualItems.map((virtualRow) => {
                const logLine = lines[virtualRow.index]
                const isStderr = logLine?.stream === 'stderr'
                return (
                  <div
                    key={virtualRow.key}
                    data-index={virtualRow.index}
                    ref={virtualizer.measureElement}
                    style={{
                      position: 'absolute',
                      top: 0,
                      left: 0,
                      width: '100%',
                      height: ROW_HEIGHT_PX,
                      transform: `translateY(${virtualRow.start}px)`,
                    }}
                    className={`truncate border-l-2 px-3 whitespace-pre ${
                      isStderr
                        ? 'border-red-500 bg-red-950/30 text-red-400'
                        : 'border-transparent text-neutral-200'
                    }`}
                  >
                    {logLine ? stripAnsiCodes(logLine.line) : ''}
                  </div>
                )
              })}
            </div>
          )}
        </div>

        {isPaused && (
          <button
            type="button"
            onClick={handleResumeClick}
            className="absolute bottom-3 left-1/2 flex -translate-x-1/2 items-center gap-1.5 rounded-full bg-neutral-800 px-3 py-1 text-xs font-medium text-neutral-100 shadow-lg hover:bg-neutral-700"
          >
            <ArrowDownIcon className="size-3.5" aria-hidden="true" />
            Resume auto-scroll
          </button>
        )}
      </div>
    </div>
  )
}

const CONNECTION_LABEL: Record<LogStreamConnectionState, string> = {
  connecting: 'Connecting...',
  open: 'Live',
  error: 'Reconnecting...',
}

// "open" and "error" now source their color from the shared
// success/warning badge variants (components/ui/badge.tsx) instead of
// duplicating bg-green-100/bg-amber-100 locally. "connecting" has no
// green/red/amber equivalent in badgeVariants, it is a neutral state, so
// it keeps its literal Tailwind classes exactly as before.
const CONNECTING_BADGE_CLASS =
  'bg-neutral-100 text-neutral-700 dark:bg-neutral-800 dark:text-neutral-300'

const CONNECTION_VARIANT: Record<
  Exclude<LogStreamConnectionState, 'connecting'>,
  'success' | 'warning'
> = {
  open: 'success',
  error: 'warning',
}

// The connection dot pulses only while genuinely live (SSE stream open):
// a still dot for "Connecting..."/"Reconnecting..." would read as "also
// live", which is the opposite of what those two states mean.
const CONNECTION_DOT_CLASS: Record<LogStreamConnectionState, string> = {
  connecting: 'bg-neutral-500 dark:bg-neutral-400',
  open: 'bg-green-600 dark:bg-green-400',
  error: 'bg-amber-600 dark:bg-amber-400',
}

function ConnectionBadge({ state }: { state: LogStreamConnectionState }) {
  return (
    <Badge
      variant={state === 'connecting' ? undefined : CONNECTION_VARIANT[state]}
      className={`gap-1.5 rounded-full ${
        state === 'connecting' ? CONNECTING_BADGE_CLASS : ''
      }`}
    >
      <span className="relative inline-flex size-2" aria-hidden="true">
        {state === 'open' ? (
          <span
            className={`absolute inline-flex size-full animate-ping rounded-full opacity-75 ${CONNECTION_DOT_CLASS[state]}`}
          />
        ) : null}
        <span
          className={`relative inline-flex size-2 rounded-full ${CONNECTION_DOT_CLASS[state]}`}
        />
      </span>
      {CONNECTION_LABEL[state]}
    </Badge>
  )
}
