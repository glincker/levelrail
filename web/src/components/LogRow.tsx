import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDownGlassIcon, ChevronRightGlassIcon } from '@thesvg/ui-react'
import { stripAnsiCodes } from '../lib/ansi'
import { splitByNeedle } from '../lib/logHighlight'
import { detectLogLevel, prettyJsonLine } from '../lib/logLevel'
import { formatLogAbsolute, formatLogRelative } from '../lib/logTime'
import { LogLevelTag } from './LogLevelTag'
import type { LogLine } from '../hooks/useLogStream'

export const LOG_ROW_HEIGHT_PX = 20

export function LogRow({
  logLine,
  index,
  start,
  expanded,
  onToggle,
  measure,
  highlight = '',
  wrap = true,
  now,
}: {
  logLine: LogLine
  index: number
  start: number
  expanded: boolean
  onToggle: () => void
  measure: (el: HTMLDivElement | null) => void
  /** Active filter text; matches inside the line are wrapped in <mark>. */
  highlight?: string
  /** Wrap long lines (default) instead of scrolling them sideways. */
  wrap?: boolean
  /** Epoch ms used for relative times; the row shows no time when omitted. */
  now?: number
}) {
  const { t } = useTranslation('common')
  const text = stripAnsiCodes(logLine.line)
  const level = detectLogLevel(logLine.line)
  const isStderr = logLine.stream === 'stderr'
  // Level-detected errors on stdout (build tools often print to stdout) get the same highlight.
  const isError = isStderr || level === 'error'
  const isSystem = logLine.stream === 'system'
  // Only JSON lines have anything more to show when expanded; parse once per line.
  const prettyAll = useMemo(() => prettyJsonLine(logLine.line), [logLine.line])
  const expandable = prettyAll !== null
  const pretty = expanded ? prettyAll : null

  // A system line (internal/api/live_logs.go's container-cutover marker)
  // is control-plane annotation, not real container output: no level tag,
  // no JSON expand, its own translated label rather than the raw wire
  // text, so a dashboard-rendered deploy cutover reads distinctly from
  // anything the app itself logged.
  if (isSystem) {
    return (
      <div
        data-index={index}
        ref={measure}
        style={{
          position: 'absolute',
          top: 0,
          left: 0,
          width: '100%',
          transform: `translateY(${start}px)`,
        }}
      >
        <div
          style={{ height: LOG_ROW_HEIGHT_PX }}
          className="flex w-full items-center gap-2 border-l-2 border-amber-600/60 bg-amber-950/20 px-3 text-left text-xs text-amber-400/90 italic"
        >
          <span aria-hidden="true">---</span>
          {t('logs.previousContainerEnded')}
        </div>
      </div>
    )
  }

  return (
    <div
      data-index={index}
      ref={measure}
      style={{
        position: 'absolute',
        top: 0,
        left: 0,
        width: '100%',
        transform: `translateY(${start}px)`,
      }}
    >
      <div
        className={`flex min-h-5 ${wrap ? 'w-full' : 'w-max min-w-full'} items-start gap-2 border-l-2 pr-3 pl-1 ${
          isError
            ? 'border-red-500 bg-red-950/30 text-red-400'
            : 'border-transparent text-neutral-200 hover:bg-neutral-900'
        }`}
      >
        <span className="flex w-5 shrink-0 items-center justify-center select-none">
          {expandable ? (
            <button
              type="button"
              onClick={onToggle}
              aria-expanded={expanded}
              aria-label={t(expanded ? 'logs.collapseJson' : 'logs.expandJson')}
              className="flex size-5 items-center justify-center rounded text-neutral-400 pointer-coarse:size-10 focus-visible:ring-1 focus-visible:ring-neutral-400 focus-visible:outline-none hover:text-neutral-100"
            >
              {expanded ? (
                <ChevronDownGlassIcon className="size-4" aria-hidden="true" />
              ) : (
                <ChevronRightGlassIcon className="size-4" aria-hidden="true" />
              )}
            </button>
          ) : null}
        </span>
        <LogLevelTag level={level} />
        {logLine.at !== undefined && now !== undefined ? (
          <time
            dateTime={new Date(logLine.at).toISOString()}
            title={formatLogAbsolute(logLine.at)}
            className="w-14 shrink-0 text-right text-xs text-neutral-500 tabular-nums select-none"
          >
            {formatLogRelative(logLine.at, now)}
          </time>
        ) : null}
        <span
          className={`flex-1 select-text ${wrap ? 'min-w-0 break-words whitespace-pre-wrap' : 'whitespace-pre'}`}
        >
          {splitByNeedle(text, highlight).map((seg, i) =>
            seg.match ? (
              <mark key={i} className="rounded-sm bg-amber-400/30 text-inherit">
                {seg.text}
              </mark>
            ) : (
              seg.text
            ),
          )}
        </span>
      </div>
      {expanded && pretty !== null ? (
        <div className="border-l-2 border-neutral-700 bg-neutral-900 px-3 py-2">
          <pre className="text-xs leading-5 break-words whitespace-pre-wrap text-neutral-200 select-text">
            {pretty}
          </pre>
        </div>
      ) : null}
    </div>
  )
}
