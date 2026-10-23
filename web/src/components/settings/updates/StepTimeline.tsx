import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  CircleNotchIcon,
  MinusCircleIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { cn } from '@/lib/utils'
import { formatElapsed } from '@/lib/upgradeTime'
import { dedupeReason } from '@/lib/upgradeAttempts'
import type { SelfUpgradeAttempt, SelfUpgradeStep } from '@/queries/selfUpgrade'

type Row =
  | SelfUpgradeStep
  | { name: string; status: 'running'; detail: string; duration_ms: number }

function StepIcon({ status }: { status: Row['status'] }) {
  if (status === 'failed') {
    return (
      <XCircleIcon
        weight="fill"
        className="size-4 text-destructive"
        aria-hidden="true"
      />
    )
  }
  if (status === 'skipped') {
    return (
      <MinusCircleIcon
        className="size-4 text-muted-foreground"
        aria-hidden="true"
      />
    )
  }
  if (status === 'running') {
    return (
      <CircleNotchIcon
        className="size-4 text-primary motion-safe:animate-spin"
        aria-hidden="true"
      />
    )
  }
  return (
    <CheckCircleIcon
      weight="fill"
      className="size-4 text-emerald-600 dark:text-emerald-400"
      aria-hidden="true"
    />
  )
}

export function StepTimeline({ attempt }: { attempt: SelfUpgradeAttempt }) {
  const { t } = useTranslation('updates')
  const rows: Row[] = [...attempt.steps]
  if (
    attempt.outcome === 'running' &&
    attempt.current_step &&
    !rows.some((s) => s.name === attempt.current_step)
  ) {
    rows.push({
      name: attempt.current_step,
      status: 'running',
      detail: '',
      duration_ms: 0,
    })
  }
  const failedIdx = rows.findIndex((s) => s.status === 'failed')
  const orphanError = failedIdx < 0 ? attempt.error : ''
  return (
    <ol>
      {rows.map((s, i) => {
        const failed = i === failedIdx
        const lines = failed
          ? dedupeReason(s.detail, attempt.error)
          : s.detail
            ? [s.detail]
            : []
        return (
          <li
            key={`${s.name}-${i}`}
            className={cn(
              'relative flex gap-3 pb-3 last:pb-0',
              failed && '-mx-2 rounded-md bg-destructive/5 p-2',
            )}
          >
            {i < rows.length - 1 ? (
              <span
                aria-hidden="true"
                className="absolute bottom-0 left-2 top-5 w-px bg-border"
              />
            ) : null}
            <span className="relative z-10 mt-0.5 flex shrink-0 bg-card">
              <StepIcon status={s.status} />
            </span>
            <div className="min-w-0 flex-1">
              <div className="flex items-baseline justify-between gap-2">
                <span
                  className={cn(
                    'text-sm',
                    s.status === 'skipped' && 'text-muted-foreground',
                    failed && 'font-medium text-destructive',
                  )}
                >
                  {t(`selfUpgrade.step.${s.name}`, { defaultValue: s.name })}
                </span>
                {s.duration_ms > 0 ? (
                  <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
                    {formatElapsed(s.duration_ms)}
                  </span>
                ) : null}
              </div>
              {lines.map((l) => (
                <p
                  key={l}
                  className="break-words text-xs text-muted-foreground"
                >
                  {l}
                </p>
              ))}
            </div>
          </li>
        )
      })}
      {orphanError ? (
        <li className="mt-2 rounded-md bg-destructive/5 p-2 text-xs text-destructive">
          {orphanError}
        </li>
      ) : null}
    </ol>
  )
}
