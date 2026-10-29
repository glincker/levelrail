import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, CopyGlassIcon } from '@thesvg/ui-react'
import { COPY_FEEDBACK_MS } from '../lib/copyFeedback'
import { stripAnsiCodes } from '../lib/ansi'
import { splitByNeedle } from '../lib/logHighlight'
import { detectLogLevel, prettyJsonLine, type LogLevel } from '../lib/logLevel'
import type { LogLine } from '../hooks/useLogStream'

export const LOG_ROW_HEIGHT_PX = 20

const LEVEL_TAG: Record<LogLevel, { label: string; className: string }> = {
  error: { label: 'ERR', className: 'text-red-400/80' },
  warn: { label: 'WRN', className: 'text-amber-400/80' },
  info: { label: 'INF', className: 'text-sky-400/70' },
  debug: { label: 'DBG', className: 'text-neutral-400' },
}

export function LogRow({
  logLine,
  index,
  start,
  expanded,
  onToggle,
  measure,
  highlight = '',
}: {
  logLine: LogLine
  index: number
  start: number
  expanded: boolean
  onToggle: () => void
  measure: (el: HTMLDivElement | null) => void
  /** Active filter text; matches inside the line are wrapped in <mark>. */
  highlight?: string
}) {
  const { t } = useTranslation('common')
  const [copied, setCopied] = useState(false)
  const timer = useRef<number | undefined>(undefined)
  useEffect(() => () => window.clearTimeout(timer.current), [])
  const text = stripAnsiCodes(logLine.line)
  const level = detectLogLevel(logLine.line)
  const tag = level ? LEVEL_TAG[level] : null
  const isStderr = logLine.stream === 'stderr'
  // Level-detected errors on stdout (build tools often print to stdout) get the same highlight.
  const isError = isStderr || level === 'error'
  const isSystem = logLine.stream === 'system'
  const pretty = expanded ? prettyJsonLine(logLine.line) : null

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
          className="flex w-full items-center gap-2 border-l-2 border-amber-600/60 bg-amber-950/20 px-3 text-left text-[11px] text-amber-400/90 italic"
        >
          <span aria-hidden="true">---</span>
          {t('logs.previousContainerEnded')}
        </div>
      </div>
    )
  }

  const handleCopy = () => {
    void navigator.clipboard.writeText(text).then(() => {
      setCopied(true)
      window.clearTimeout(timer.current)
      timer.current = window.setTimeout(() => {
        setCopied(false)
      }, COPY_FEEDBACK_MS)
    })
  }

  return (
    <div
      data-index={index}
      ref={measure}
      className="group"
      style={{
        position: 'absolute',
        top: 0,
        left: 0,
        width: '100%',
        transform: `translateY(${start}px)`,
      }}
    >
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={expanded}
        style={{ height: LOG_ROW_HEIGHT_PX }}
        className={`flex w-full items-center gap-2 border-l-2 px-3 text-left whitespace-pre focus-visible:bg-neutral-900 focus-visible:ring-1 focus-visible:ring-neutral-400 focus-visible:outline-none focus-visible:ring-inset ${
          isError
            ? 'border-red-500 bg-red-950/30 text-red-400'
            : 'border-transparent text-neutral-200 hover:bg-neutral-900'
        }`}
      >
        <span
          className={`w-7 shrink-0 text-[10px] font-semibold ${tag?.className ?? ''}`}
        >
          <span aria-hidden="true">{tag?.label ?? ''}</span>
          {level ? <span className="sr-only">{level}</span> : null}
        </span>
        <span className="truncate">
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
      </button>
      <button
        type="button"
        onClick={handleCopy}
        aria-label={t('logs.copyLineAria')}
        className="absolute -top-0.5 right-2 flex h-6 min-w-6 items-center justify-center rounded bg-neutral-800 px-1.5 text-neutral-300 opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 [@media(hover:none)]:opacity-100 focus-visible:opacity-100 focus-visible:ring-1 focus-visible:ring-neutral-400 focus-visible:outline-none hover:text-neutral-100"
      >
        {copied ? (
          <CheckGlassIcon className="size-3.5" aria-hidden="true" />
        ) : (
          <CopyGlassIcon className="size-3.5" aria-hidden="true" />
        )}
      </button>
      {expanded ? (
        <div className="border-l-2 border-neutral-700 bg-neutral-900 px-3 py-2">
          <pre className="text-xs leading-5 break-words whitespace-pre-wrap text-neutral-200">
            {pretty ?? text}
          </pre>
          <button
            type="button"
            onClick={handleCopy}
            className="mt-1 flex min-h-6 items-center gap-1 rounded px-1.5 py-0.5 text-[11px] focus-visible:ring-1 focus-visible:ring-neutral-400 focus-visible:outline-none text-neutral-400 hover:bg-neutral-800 hover:text-neutral-100"
          >
            {copied ? (
              <CheckGlassIcon className="size-3.5" aria-hidden="true" />
            ) : (
              <CopyGlassIcon className="size-3.5" aria-hidden="true" />
            )}
            {copied ? t('logs.copied') : t('logs.copyLine')}
          </button>
        </div>
      ) : null}
      {copied ? (
        <span role="status" className="sr-only">
          {t('logs.lineCopied')}
        </span>
      ) : null}
    </div>
  )
}
