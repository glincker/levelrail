import { useState } from 'react'
import { CheckIcon, CopyIcon } from '@phosphor-icons/react/dist/ssr'
import { stripAnsiCodes } from '../lib/ansi'
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
}: {
  logLine: LogLine
  index: number
  start: number
  expanded: boolean
  onToggle: () => void
  measure: (el: HTMLDivElement | null) => void
}) {
  const [copied, setCopied] = useState(false)
  const text = stripAnsiCodes(logLine.line)
  const level = detectLogLevel(logLine.line)
  const tag = level ? LEVEL_TAG[level] : null
  const isStderr = logLine.stream === 'stderr'
  const pretty = expanded ? prettyJsonLine(logLine.line) : null

  const handleCopy = () => {
    void navigator.clipboard.writeText(text).then(() => {
      setCopied(true)
      window.setTimeout(() => {
        setCopied(false)
      }, 1500)
    })
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
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={expanded}
        style={{ height: LOG_ROW_HEIGHT_PX }}
        className={`flex w-full items-center gap-2 border-l-2 px-3 text-left whitespace-pre focus-visible:bg-neutral-900 focus-visible:ring-1 focus-visible:ring-neutral-400 focus-visible:outline-none focus-visible:ring-inset ${
          isStderr
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
        <span className="truncate">{text}</span>
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
              <CheckIcon className="size-3" aria-hidden="true" />
            ) : (
              <CopyIcon className="size-3" aria-hidden="true" />
            )}
            {copied ? 'Copied' : 'Copy line'}
          </button>
          <span role="status" className="sr-only">
            {copied ? 'Line copied' : ''}
          </span>
        </div>
      ) : null}
    </div>
  )
}
