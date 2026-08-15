import { useCallback, useEffect, useRef } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { ArrowDownIcon } from '@phosphor-icons/react/dist/ssr'
import { useLogStream } from '../hooks/useLogStream'
import { buildLiveLogStreamUrl } from '../queries/liveLogs'
import { stripAnsiCodes } from '../lib/ansi'
import { LogConnectionBadge } from './LogConnectionBadge'

// Live-tailing terminal view for one app's running container output:
// GET /api/v1/apps/{name}/logs/stream (internal/api/live_logs.go), which
// backfills a short window of recent context before switching to a live
// tail (see that handler's own doc comment for the backfill-to-live
// handoff). Structurally this is the app-log sibling of
// routes/apps/$name/deploys/$deployId/logs.tsx's build-log viewer: same
// hook (useLogStream), same virtualized monospace terminal look, same
// auto-scroll/pause behavior, because it's the same kind of problem
// (tail an append-only, possibly high-volume text stream without
// yanking the reader's scroll position). Kept as a separate component
// rather than folded into that route file: that route owns its own
// full-page header and has no search/live toggle to coordinate with,
// this one is embedded inside routes/apps/$name/logs.tsx alongside
// LogSearchPanel.
//
// stripAnsiCodes is reused for the same reason it exists there: an
// app's own stdout/stderr is exactly as likely to carry ANSI color
// codes (many web frameworks colorize their own startup/request logs)
// as build tool output is.

const ROW_HEIGHT_PX = 20
// How close to the bottom (px) still counts as "at the bottom" for
// auto-scroll purposes. scrollHeight/clientHeight are subject to
// sub-pixel rounding, so an exact-zero comparison would flap.
const BOTTOM_THRESHOLD_PX = 32

export function LiveLogViewer({ appName }: { appName: string }) {
  const url = buildLiveLogStreamUrl(appName)
  const { lines, connectionState, isPaused, pause, resume } = useLogStream(url)

  const parentRef = useRef<HTMLDivElement>(null)
  // Distinguishes a scroll event caused by the auto-scroll effect below
  // from one caused by the user dragging the scrollbar/wheel/trackpad,
  // so the scroll handler does not immediately re-pause right after an
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
  // isPaused. This is the exact behavior every real log tail (kubectl
  // logs -f, Railway, Coolify) gives you, and the one most naive
  // implementations get wrong: scrolling up to read an earlier line
  // must not get yanked back to the bottom by the next line arriving.
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
    <div className="flex h-full flex-col gap-2">
      <div className="flex items-center justify-between gap-3">
        <p className="text-xs text-muted-foreground">
          {lines.length > 0
            ? `${lines.length.toLocaleString()} lines (recent context, then live)`
            : 'Waiting for log output...'}
        </p>
        <LogConnectionBadge state={connectionState} />
      </div>

      <div className="relative flex-1">
        <div
          ref={parentRef}
          onScroll={handleScroll}
          className="h-[65vh] overflow-auto rounded-lg border border-neutral-800 bg-neutral-950 font-mono text-xs leading-5 text-neutral-200"
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
