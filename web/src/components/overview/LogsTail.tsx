import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { CaretDownIcon, CaretRightIcon } from '@phosphor-icons/react/dist/ssr'
import { useIsMobile } from '../../hooks/use-mobile'
import { useLogStream } from '../../hooks/useLogStream'
import { LEVEL_DOT, levelOf } from './logLevels'
import { stripAnsiCodes } from '../../lib/ansi'
import { buildLiveLogStreamUrl } from '../../queries/liveLogs'

const TAIL_LINES = 6

function TailBody({ appName }: { appName: string }) {
  const { lines, connectionState } = useLogStream(
    buildLiveLogStreamUrl(appName),
  )
  const tail = lines.slice(-TAIL_LINES)
  if (tail.length === 0) {
    return (
      <p className="px-3 py-2 text-xs text-muted-foreground">
        {connectionState === 'error'
          ? 'Log stream unavailable.'
          : 'Waiting for log output...'}
      </p>
    )
  }
  return (
    <ul className="space-y-0.5 px-3 py-2 font-mono text-xs">
      {tail.map((l) => (
        <li key={l.id} className="flex items-start gap-2">
          <span
            aria-hidden="true"
            className={`mt-1.5 size-1.5 shrink-0 rounded-full ${LEVEL_DOT[levelOf(l)]}`}
          />
          <span className="min-w-0 truncate">{stripAnsiCodes(l.line)}</span>
        </li>
      ))}
    </ul>
  )
}

export function LogsTail({ appName }: { appName: string }) {
  const isMobile = useIsMobile()
  const [override, setOverride] = useState<boolean | null>(null)
  const open = override ?? !isMobile

  return (
    <section
      aria-label="Live logs"
      className="rounded-xl border border-border bg-card"
    >
      <div className="flex items-center justify-between px-3 py-2">
        <button
          type="button"
          aria-expanded={open}
          onClick={() => setOverride(!open)}
          className="flex items-center gap-1.5 text-sm font-medium"
        >
          {open ? (
            <CaretDownIcon className="size-3.5" aria-hidden="true" />
          ) : (
            <CaretRightIcon className="size-3.5" aria-hidden="true" />
          )}
          Live logs
        </button>
        <Link
          to="/apps/$name/logs"
          params={{ name: appName }}
          className="text-xs text-primary underline-offset-2 hover:underline"
        >
          Open logs
        </Link>
      </div>
      {open ? <TailBody appName={appName} /> : null}
    </section>
  )
}
