import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { StatusPill, type Tone } from '@/components/kit'
import {
  selfUpgradeAttemptsQueryOptions,
  type AttemptOutcome,
  type SelfUpgradeAttempt,
} from '../../queries/selfUpgrade'

const OUTCOME_TONE: Record<AttemptOutcome, Tone> = {
  running: 'info',
  succeeded: 'success',
  rolled_back: 'warning',
  refused: 'neutral',
  failed: 'danger',
}

export function AttemptRow({ attempt }: { attempt: SelfUpgradeAttempt }) {
  const { t } = useTranslation('updates')
  return (
    <li className="space-y-1.5 rounded-md border border-border px-3 py-2">
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <StatusPill
          tone={OUTCOME_TONE[attempt.outcome]}
          label={t(`selfUpgrade.outcome.${attempt.outcome}`)}
          size="sm"
        />
        <span className="font-mono text-xs">
          {attempt.from_version} {'->'} {attempt.to_version}
        </span>
        <span className="text-xs text-muted-foreground">
          {t('selfUpgrade.by', { who: attempt.initiator || '-' })}
        </span>
      </div>
      <ol className="space-y-0.5 text-xs">
        {attempt.steps.map((s, i) => (
          <li key={`${s.name}-${i}`} className="flex gap-2">
            <span
              className={
                s.status === 'failed'
                  ? 'text-destructive'
                  : s.status === 'skipped'
                    ? 'text-muted-foreground'
                    : 'text-foreground'
              }
            >
              {t(`selfUpgrade.step.${s.name}`, { defaultValue: s.name })}
            </span>
            <span className="min-w-0 flex-1 truncate text-muted-foreground">
              {s.detail}
            </span>
          </li>
        ))}
      </ol>
      {attempt.error ? (
        <p className="text-xs text-destructive">{attempt.error}</p>
      ) : null}
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
    </li>
  )
}

export function UpgradeAttempts() {
  const { t } = useTranslation('updates')
  const attempts = useQuery(selfUpgradeAttemptsQueryOptions(false))
  const list = attempts.data ?? []
  if (list.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        {t('selfUpgrade.noAttempts')}
      </p>
    )
  }
  return (
    <ul className="space-y-2" aria-label={t('selfUpgrade.attemptsTitle')}>
      {list.map((a) => (
        <AttemptRow key={a.id} attempt={a} />
      ))}
    </ul>
  )
}
