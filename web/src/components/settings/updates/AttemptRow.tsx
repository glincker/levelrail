import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowRightGlassIcon, ChevronRightGlassIcon } from '@thesvg/ui-react'
import { StatusPill, type Tone } from '@/components/kit'
import { cn } from '@/lib/utils'
import { attemptDurationMs } from '@/lib/upgradeAttempts'
import { formatElapsed } from '@/lib/upgradeTime'
import { formatAbsoluteTime, formatRelativeTime } from '@/lib/relativeTime'
import type { AttemptOutcome, SelfUpgradeAttempt } from '@/queries/selfUpgrade'
import { VersionLabel } from './VersionLabel'
import { StepTimeline } from './StepTimeline'
import { FailureActions } from './FailureActions'

const OUTCOME_TONE: Record<AttemptOutcome, Tone> = {
  running: 'info',
  succeeded: 'success',
  rolled_back: 'warning',
  refused: 'neutral',
  failed: 'danger',
}

export function AttemptRow({
  attempt,
  defaultOpen = false,
}: {
  attempt: SelfUpgradeAttempt
  defaultOpen?: boolean
}) {
  const { t } = useTranslation('updates')
  const [open, setOpen] = useState(defaultOpen)
  const panelId = useId()
  const duration = attemptDurationMs(attempt)
  return (
    <li className="rounded-md border border-border">
      <button
        type="button"
        aria-expanded={open}
        aria-controls={panelId}
        onClick={() => setOpen((v) => !v)}
        className="flex w-full flex-wrap items-center gap-x-3 gap-y-1 rounded-md px-3 py-2 text-left hover:bg-muted/50 focus-visible:outline-2 focus-visible:outline-ring"
      >
        <ChevronRightGlassIcon
          className={cn(
            'size-4 shrink-0 text-muted-foreground transition-transform motion-reduce:transition-none',
            open && 'rotate-90',
          )}
          aria-hidden="true"
        />
        <StatusPill
          tone={OUTCOME_TONE[attempt.outcome]}
          label={t(`selfUpgrade.outcome.${attempt.outcome}`)}
          size="sm"
          live={attempt.outcome === 'running'}
        />
        <span className="flex min-w-0 flex-wrap items-center gap-1.5">
          <VersionLabel value={attempt.from_version} />
          <ArrowRightGlassIcon
            className="size-4 shrink-0 text-muted-foreground"
            aria-label={t('selfUpgrade.to')}
          />
          <VersionLabel value={attempt.to_version} />
        </span>
        <span className="ml-auto flex items-center gap-2 text-xs text-muted-foreground">
          <time
            dateTime={attempt.started_at}
            title={formatAbsoluteTime(attempt.started_at)}
          >
            {formatRelativeTime(attempt.started_at)}
          </time>
          {duration > 0 ? (
            <span className="tabular-nums">{formatElapsed(duration)}</span>
          ) : null}
        </span>
      </button>
      <div
        id={panelId}
        hidden={!open}
        className="space-y-3 border-t border-border px-3 py-3"
      >
        <p className="text-xs text-muted-foreground">
          {t('selfUpgrade.by', {
            who: attempt.initiator || t('selfUpgrade.unknownActor'),
          })}
        </p>
        <StepTimeline attempt={attempt} />
        {attempt.failure ? <FailureActions attempt={attempt} /> : null}
        {attempt.outcome === 'rolled_back' ? (
          <p className="text-xs text-muted-foreground">
            {t('selfUpgrade.rolledBackNote', { version: attempt.from_version })}{' '}
            {attempt.database_restored
              ? t('selfUpgrade.dbRestored', {
                  minutes: attempt.rolled_back_minutes ?? 0,
                })
              : t('selfUpgrade.dbKept')}
          </p>
        ) : null}
      </div>
    </li>
  )
}
