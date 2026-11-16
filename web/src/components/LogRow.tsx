import { Fragment, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ChevronDownGlassIcon,
  ChevronRightGlassIcon,
  CircleAlertGlassIcon,
} from '@thesvg/ui-react'
import {
  DEFAULT_ANSI_STYLE,
  ansiClassName,
  parseAnsiLine,
  type AnsiStyle,
} from '../lib/ansiRender'
import { highlightStyled } from '../lib/logHighlight'
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
  ansiStart = DEFAULT_ANSI_STYLE,
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
  /** Colour carried over from earlier lines in the stream (ANSI state persists until reset). */
  ansiStart?: AnsiStyle
}) {
  const { t } = useTranslation('common')
  const pieces = useMemo(
    () =>
      highlightStyled(
        parseAnsiLine(logLine.line, ansiStart).segments,
        highlight,
      ),
    [logLine.line, ansiStart, highlight],
  )
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
          className="flex w-full items-center gap-2 border-l-2 border-glinr-code-log-warn bg-glinr-code-log-warn-bg px-3 text-left text-xs text-glinr-code-log-warn italic"
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
            ? 'border-glinr-code-log-error-border bg-glinr-code-log-error-bg text-glinr-code-log-error'
            : 'border-transparent text-glinr-code-fg hover:bg-glinr-code-line-highlight'
        }`}
      >
        <span className="flex w-5 shrink-0 items-center justify-center select-none">
          {expandable ? (
            <button
              type="button"
              onClick={onToggle}
              aria-expanded={expanded}
              aria-label={t(expanded ? 'logs.collapseJson' : 'logs.expandJson')}
              className="flex size-5 items-center justify-center rounded text-glinr-code-muted pointer-coarse:size-10 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none hover:text-glinr-code-fg"
            >
              {expanded ? (
                <ChevronDownGlassIcon className="size-4" aria-hidden="true" />
              ) : (
                <ChevronRightGlassIcon className="size-4" aria-hidden="true" />
              )}
            </button>
          ) : isError ? (
            <CircleAlertGlassIcon
              className="size-3.5 text-glinr-code-log-error"
              aria-hidden="true"
            />
          ) : null}
        </span>
        <LogLevelTag level={level} />
        {logLine.at !== undefined && now !== undefined ? (
          <time
            dateTime={new Date(logLine.at).toISOString()}
            title={formatLogAbsolute(logLine.at)}
            className="w-14 shrink-0 text-right text-xs text-glinr-code-log-timestamp tabular-nums select-none"
          >
            {formatLogRelative(logLine.at, now)}
          </time>
        ) : null}
        <span
          className={`flex-1 select-text ${wrap ? 'min-w-0 break-words whitespace-pre-wrap' : 'whitespace-pre'}`}
        >
          {pieces.map((p, i) => {
            const cls = ansiClassName(p.style)
            const node = cls ? <span className={cls}>{p.text}</span> : p.text
            return p.match ? (
              <mark
                key={i}
                className="rounded-sm bg-glinr-code-match text-inherit"
              >
                {node}
              </mark>
            ) : (
              <Fragment key={i}>{node}</Fragment>
            )
          })}
        </span>
      </div>
      {expanded && pretty !== null ? (
        <div className="border-l-2 border-glinr-code-border bg-glinr-code-line-highlight px-3 py-2">
          <pre className="text-xs leading-5 break-words whitespace-pre-wrap text-glinr-code-fg select-text">
            {pretty}
          </pre>
        </div>
      ) : null}
    </div>
  )
}
