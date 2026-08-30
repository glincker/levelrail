import { useCallback, useEffect, useRef, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import {
  ArrowDownIcon,
  ArrowsInIcon,
  ArrowsOutIcon,
} from '@phosphor-icons/react/dist/ssr'
import { stripAnsiCodes } from '../lib/ansi'
import type { LogLine } from '../hooks/useLogStream'
import { Dialog, DialogContent, DialogTitle } from './ui/dialog'

const ROW_HEIGHT_PX = 20
// How close to the bottom (px) still counts as "at the bottom" for
// auto-scroll purposes. scrollHeight/clientHeight are subject to
// sub-pixel rounding, so an exact-zero comparison would flap.
const BOTTOM_THRESHOLD_PX = 32

// The virtualized terminal window both SSE log views (routes/apps/$name/
// logs.tsx's live tab and routes/apps/$name/deploys/$deployId/logs.tsx)
// render, once each already has its own useLogStream(url) connection:
// same virtualizer setup, same scroll-follow/pause logic, same row
// rendering, same resume button, originally two separate, near-identical
// copies of this exact body. Deliberately takes lines/isPaused/pause/
// resume as props rather than a url and calling useLogStream itself: the
// two callers' status rows above this component differ in real, not just
// cosmetic ways (the embedded app-logs panel wants a single line count,
// the deploy-logs route wants a full page header naming the app and
// deploy), and both already need the same useLogStream data to build
// those headers. Owning the hook call here too would mean either a
// second, wasteful EventSource connection to the same endpoint just for
// the header, or this component reaching up into its caller's header via
// a render prop for no real gain. Staying purely presentational (given
// the stream's data, render and scroll-follow it) is the actual
// boundary, not "owns everything log-stream-related."
export function LogTerminal({
  lines,
  isPaused,
  pause,
  resume,
  heightClassName = 'h-[65vh]',
  emptyStateMessage = 'Waiting for log output...',
  emptyStatePulse = true,
  isFinished = false,
}: {
  lines: LogLine[]
  isPaused: boolean
  pause: () => void
  resume: () => void
  heightClassName?: string
  emptyStateMessage?: string
  // False for a state that will never resolve into output (e.g. an
  // image-sourced deploy, which has no build step to wait on): the
  // pulsing dot otherwise implies output is still coming.
  emptyStatePulse?: boolean
  // True once the underlying process (e.g. a deploy attempt) has reached
  // a terminal state. Stops force-scrolling on new lines even if the
  // user never manually scrolled away from the bottom, so a user reading
  // historical output isn't yanked to the bottom after the process is
  // already done. Live app/database log streams never finish, so callers
  // for those simply omit this.
  isFinished?: boolean
}) {
  const [isFullscreen, setIsFullscreen] = useState(false)
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
    if (isPaused || isFinished || lines.length === 0) {
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
  }, [lines.length, isPaused, isFinished, virtualizer])

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

  const body = (
    <div
      className={
        isFullscreen
          ? 'relative flex min-h-0 flex-1 flex-col'
          : 'relative flex-1'
      }
    >
      <div
        ref={parentRef}
        onScroll={handleScroll}
        className={`${isFullscreen ? 'h-full' : heightClassName} overflow-auto rounded-lg border border-neutral-800 bg-neutral-950 font-mono text-xs leading-5 text-neutral-200`}
      >
        {lines.length === 0 ? (
          <p className="flex items-center gap-2 px-3 py-2 text-neutral-500">
            <span
              className={`size-1.5 rounded-full bg-neutral-500 ${emptyStatePulse ? 'animate-pulse' : ''}`}
            />
            {emptyStateMessage}
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

      <button
        type="button"
        onClick={() => {
          setIsFullscreen((prev) => !prev)
        }}
        aria-label={isFullscreen ? 'Exit fullscreen' : 'View fullscreen'}
        className="absolute top-2 right-2 rounded p-1 text-neutral-400 hover:bg-neutral-800 hover:text-neutral-100"
      >
        {isFullscreen ? (
          <ArrowsInIcon className="size-3.5" aria-hidden="true" />
        ) : (
          <ArrowsOutIcon className="size-3.5" aria-hidden="true" />
        )}
      </button>

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
  )

  if (!isFullscreen) {
    return body
  }

  return (
    <Dialog
      open
      onOpenChange={(next) => {
        if (!next) {
          setIsFullscreen(false)
        }
      }}
    >
      <DialogContent
        size="fullscreen"
        showCloseButton={false}
        className="flex flex-col"
      >
        <DialogTitle className="sr-only">Log output</DialogTitle>
        {body}
      </DialogContent>
    </Dialog>
  )
}
