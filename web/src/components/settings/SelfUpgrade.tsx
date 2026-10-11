import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { StatusPill, type Tone } from '@/components/kit'
import { Alert, AlertDescription, AlertTitle } from '../ui/alert'
import { Button } from '../ui/button'
import { Checkbox } from '../ui/checkbox'
import {
  selfUpgradeAttemptsQueryOptions,
  selfUpgradePlanQueryOptions,
  useStartSelfUpgrade,
  type AttemptOutcome,
  type SelfUpgradeAttempt,
} from '../../queries/selfUpgrade'
import { CopyCommand } from './CopyCommand'

const OUTCOME_TONE: Record<AttemptOutcome, Tone> = {
  running: 'info',
  succeeded: 'success',
  rolled_back: 'warning',
  refused: 'neutral',
  failed: 'danger',
}

const POLL_MS = 3_000

function AttemptRow({ attempt }: { attempt: SelfUpgradeAttempt }) {
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
          {t('selfUpgrade.rolledBackNote', { version: attempt.from_version })}
        </p>
      ) : null}
    </li>
  )
}

export function SelfUpgrade() {
  const { t } = useTranslation('updates')
  const plan = useQuery(selfUpgradePlanQueryOptions(''))
  const [acked, setAcked] = useState<Record<string, boolean>>({})
  const start = useStartSelfUpgrade()
  const hasRunning = (list: SelfUpgradeAttempt[] | undefined) =>
    (list ?? []).some((a) => a.outcome === 'running')
  const attempts = useQuery({
    ...selfUpgradeAttemptsQueryOptions(false),
    refetchInterval: (q) =>
      start.isSuccess || hasRunning(q.state.data) ? POLL_MS : false,
  })

  if (plan.isPending) {
    return (
      <p className="text-sm text-muted-foreground">
        {t('selfUpgrade.loading')}
      </p>
    )
  }
  const data = plan.data
  const needAck = data?.breaking.filter((b) => b.requires_ack) ?? []
  const allAcked = needAck.every((b) => acked[b.id])

  return (
    <div className="space-y-4">
      {plan.isError || !data ? (
        <p className="text-sm text-muted-foreground">
          {t('selfUpgrade.noPlan')}
        </p>
      ) : (
        <div className="space-y-3">
          <p className="text-sm">
            {t('selfUpgrade.target', {
              current: data.current_version,
              target: data.target_version,
            })}
          </p>
          {!data.notes_available ? (
            <Alert>
              <AlertTitle>{t('selfUpgrade.notesMissingTitle')}</AlertTitle>
              <AlertDescription>
                {t('selfUpgrade.notesMissing')}
              </AlertDescription>
            </Alert>
          ) : null}
          {data.breaking.length > 0 ? (
            <div className="space-y-2">
              <p className="text-sm font-medium">
                {t('selfUpgrade.breakingTitle')}
              </p>
              <ul className="space-y-2">
                {data.breaking.map((b) => (
                  <li
                    key={b.id}
                    className="flex items-start gap-2 rounded-md bg-muted px-3 py-2 text-sm"
                  >
                    {b.requires_ack ? (
                      <Checkbox
                        id={`ack-${b.id}`}
                        checked={acked[b.id] === true}
                        onCheckedChange={(v) =>
                          setAcked((prev) => ({ ...prev, [b.id]: v === true }))
                        }
                      />
                    ) : null}
                    <label htmlFor={`ack-${b.id}`} className="min-w-0 flex-1">
                      <span className="font-mono text-xs text-muted-foreground">
                        {b.version}
                      </span>{' '}
                      {b.summary}
                      {b.requires_ack ? (
                        <span className="block text-xs text-muted-foreground">
                          {t('selfUpgrade.ackRequired')}
                        </span>
                      ) : null}
                    </label>
                  </li>
                ))}
              </ul>
            </div>
          ) : data.notes_available ? (
            <p className="text-sm text-muted-foreground">
              {t('selfUpgrade.noBreaking')}
            </p>
          ) : null}
          <p className="text-xs text-muted-foreground">
            {t('selfUpgrade.safety')}
          </p>
          {data.can_apply ? (
            <div className="flex flex-wrap items-center gap-3">
              <Button
                disabled={
                  start.isPending ||
                  !data.notes_available ||
                  !allAcked ||
                  hasRunning(attempts.data)
                }
                onClick={() =>
                  start.mutate({
                    target: data.target_version,
                    ack: needAck.filter((b) => acked[b.id]).map((b) => b.id),
                  })
                }
              >
                {start.isPending
                  ? t('selfUpgrade.starting')
                  : t('selfUpgrade.apply', { version: data.target_version })}
              </Button>
              {!allAcked ? (
                <span className="text-xs text-muted-foreground">
                  {t('selfUpgrade.ackFirst')}
                </span>
              ) : null}
            </div>
          ) : (
            <div className="space-y-2">
              <p className="text-sm text-muted-foreground">
                {t('selfUpgrade.hostOnly', {
                  reason: data.cannot_apply_reason ?? '',
                })}
              </p>
              <CopyCommand
                label={t('selfUpgrade.hostCommand')}
                command={data.command}
              />
            </div>
          )}
          {start.isError ? (
            <p role="alert" className="text-sm text-destructive">
              {start.error.message}
            </p>
          ) : null}
          {start.isSuccess ? (
            <p role="status" className="text-sm text-muted-foreground">
              {t('selfUpgrade.started')}
            </p>
          ) : null}
        </div>
      )}
      <div className="space-y-2">
        <p className="text-sm font-medium">{t('selfUpgrade.attemptsTitle')}</p>
        {attempts.data && attempts.data.length > 0 ? (
          <ul className="space-y-2" aria-label={t('selfUpgrade.attemptsTitle')}>
            {attempts.data.map((a) => (
              <AttemptRow key={a.id} attempt={a} />
            ))}
          </ul>
        ) : (
          <p className="text-sm text-muted-foreground">
            {t('selfUpgrade.noAttempts')}
          </p>
        )}
      </div>
    </div>
  )
}
