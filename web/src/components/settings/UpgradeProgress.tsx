import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  CircleIcon,
  MinusCircleIcon,
  SpinnerGapIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { SelfUpgradeAttempt } from '../../queries/selfUpgrade'
import { progressRows, type RowState } from './upgradeProgressRows'

function StateIcon({ state }: { state: RowState }) {
  const cls = 'size-4 shrink-0'
  switch (state) {
    case 'running':
      return (
        <SpinnerGapIcon
          className={`${cls} animate-spin text-primary`}
          aria-hidden
        />
      )
    case 'ok':
      return <CheckCircleIcon className={`${cls} text-success`} aria-hidden />
    case 'failed':
      return <XCircleIcon className={`${cls} text-destructive`} aria-hidden />
    case 'skipped':
      return (
        <MinusCircleIcon
          className={`${cls} text-muted-foreground`}
          aria-hidden
        />
      )
    default:
      return (
        <CircleIcon className={`${cls} text-muted-foreground`} aria-hidden />
      )
  }
}

function formatElapsed(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000))
  const m = Math.floor(total / 60)
  const s = total % 60
  return m > 0 ? `${m}m ${s}s` : `${s}s`
}

function useNow(active: boolean): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!active) return
    const id = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(id)
  }, [active])
  return now
}

interface Props {
  target: string
  plannedSteps: string[]
  attempt: SelfUpgradeAttempt | undefined
  startedAt: number | null
  reconnecting: boolean
  releaseUrl: string | null | undefined
  onDismiss?: () => void
}

// UpgradeProgress is the live run view: ordered steps with state, elapsed
// time, an expandable log and a clear end state. It renders from the
// persisted attempt, so a reload or a control plane restart resumes it.
export function UpgradeProgress({
  target,
  plannedSteps,
  attempt,
  startedAt,
  reconnecting,
  releaseUrl,
  onDismiss,
}: Props) {
  const { t } = useTranslation('updates')
  const outcome = attempt?.outcome ?? 'running'
  const running = outcome === 'running'
  const now = useNow(running)
  const begin = attempt ? Date.parse(attempt.started_at) : (startedAt ?? now)
  const end =
    !running && attempt?.finished_at ? Date.parse(attempt.finished_at) : now
  const rows = progressRows(plannedSteps, attempt)
  const log = attempt?.log ?? []

  return (
    <section
      className="space-y-3 rounded-md border border-border p-4"
      role="status"
      aria-live="polite"
    >
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <p className="text-sm font-medium">
          {running
            ? t('progress.running', { version: target })
            : t(`progress.end.${outcome}`, { version: target })}
        </p>
        <span className="font-mono text-xs text-muted-foreground">
          {t('progress.elapsed', { time: formatElapsed(end - begin) })}
        </span>
      </div>

      {running && reconnecting ? (
        <p className="text-xs text-muted-foreground">
          {t('progress.reconnecting')}
        </p>
      ) : null}
      {running && !attempt ? (
        <p className="text-xs text-muted-foreground">{t('progress.waiting')}</p>
      ) : null}

      <ol className="space-y-1.5">
        {rows.map((r) => (
          <li key={r.name} className="flex items-start gap-2 text-sm">
            <StateIcon state={r.state} />
            <span
              className={
                r.state === 'pending'
                  ? 'text-muted-foreground'
                  : r.state === 'failed'
                    ? 'text-destructive'
                    : 'text-foreground'
              }
            >
              {t(`selfUpgrade.step.${r.name}`, { defaultValue: r.name })}
            </span>
            {r.durationMs > 0 ? (
              <span className="text-xs text-muted-foreground">
                {formatElapsed(r.durationMs)}
              </span>
            ) : null}
            {r.detail ? (
              <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground">
                {r.detail}
              </span>
            ) : null}
          </li>
        ))}
      </ol>

      {log.length > 0 ? (
        <details className="text-xs" open={running}>
          <summary className="cursor-pointer text-muted-foreground">
            {t('progress.log')}
          </summary>
          <pre className="mt-2 max-h-56 overflow-auto rounded-md bg-muted p-2 font-mono leading-relaxed">
            {log
              .map(
                (l) =>
                  `${l.at.slice(11, 19)} ${l.step ? `[${l.step}] ` : ''}${l.line}`,
              )
              .join('\n')}
          </pre>
        </details>
      ) : null}

      {outcome === 'succeeded' ? (
        <div className="space-y-1 text-sm">
          <p>{t('progress.successBody', { version: target })}</p>
          {releaseUrl ? (
            <a
              className="text-primary underline underline-offset-2"
              href={releaseUrl}
              target="_blank"
              rel="noreferrer"
            >
              {t('progress.releaseNotes')}
            </a>
          ) : null}
        </div>
      ) : null}
      {outcome !== 'running' && outcome !== 'succeeded' && attempt ? (
        <div className="space-y-1 text-sm">
          {attempt.failed_step ? (
            <p>
              {t('progress.failedAt', {
                step: t(`selfUpgrade.step.${attempt.failed_step}`, {
                  defaultValue: attempt.failed_step,
                }),
              })}
            </p>
          ) : null}
          {attempt.error ? (
            <p className="break-words text-destructive">{attempt.error}</p>
          ) : null}
          <p className="text-muted-foreground">
            {outcome === 'rolled_back'
              ? t('progress.rolledBack', { version: attempt.from_version })
              : t('progress.untouched', { version: attempt.from_version })}
          </p>
          <p className="text-muted-foreground">{t('progress.nextAction')}</p>
        </div>
      ) : null}
      {!running && onDismiss ? (
        <button
          type="button"
          className="text-xs text-muted-foreground underline underline-offset-2"
          onClick={onDismiss}
        >
          {t('progress.dismiss')}
        </button>
      ) : null}
    </section>
  )
}
