import '../../../styles/upgradeMotion.css'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CircleCheckGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
import type { SelfUpgradeAttempt } from '../../../queries/selfUpgrade'
import { buildPipeline, type PhaseId } from '../../../lib/upgradePhases'
import { formatElapsed, useNow } from '../../../lib/upgradeTime'
import { usePrefersReducedMotion } from '../../../hooks/usePrefersReducedMotion'
import { UpgradePhaseDetail } from './UpgradePhaseDetail'
import { UpgradeStageNode } from './UpgradeStageNode'
import { UpgradeSuccessBurst } from './UpgradeSuccessBurst'

interface Props {
  target: string
  plannedSteps: string[]
  attempt: SelfUpgradeAttempt | undefined
  startedAt: number | null
  reconnecting: boolean
  showReload?: boolean
}

function connectorClass(
  prevOk: boolean,
  nextRunning: boolean,
  motion: boolean,
): string {
  if (prevOk && nextRunning) {
    return cn(
      'bg-gradient-to-r from-success via-primary to-primary',
      motion && 'upg-sweep',
    )
  }
  return prevOk ? 'bg-success' : 'bg-border'
}

export function UpgradePipeline({
  target,
  plannedSteps,
  attempt,
  startedAt,
  reconnecting,
  showReload = true,
}: Props) {
  const { t } = useTranslation('updates')
  const motion = !usePrefersReducedMotion()
  const panelId = useId()
  const outcome = attempt?.outcome ?? 'running'
  const running = outcome === 'running'
  const now = useNow(running)
  const view = buildPipeline(plannedSteps, attempt)
  const [picked, setPicked] = useState<PhaseId | null>(null)
  const auto =
    view.phases.find((p) => p.state === 'failed' || p.state === 'running') ??
    [...view.phases].reverse().find((p) => p.state === 'ok') ??
    view.phases[0]
  const open = view.phases.find((p) => p.id === picked) ?? auto
  const begin = attempt ? Date.parse(attempt.started_at) : (startedAt ?? now)
  const end =
    !running && attempt?.finished_at ? Date.parse(attempt.finished_at) : now

  return (
    <section className="space-y-4 rounded-md border border-border p-4">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <p className="text-sm font-medium" role="status" aria-live="polite">
          {running
            ? t('progress.running', { version: target })
            : t(`progress.end.${outcome}`, { version: target })}
        </p>
        <span className="font-mono text-xs text-muted-foreground">
          {t('progress.elapsed', { time: formatElapsed(end - begin) })}
        </span>
      </div>
      {running && reconnecting ? (
        <p
          className={cn(
            'text-xs text-muted-foreground',
            motion && 'upg-breathe',
          )}
        >
          {t('pipeline.reconnecting')}
        </p>
      ) : null}
      {running && !attempt ? (
        <p className="text-xs text-muted-foreground">{t('progress.waiting')}</p>
      ) : null}

      <ol className="flex flex-col sm:flex-row sm:items-start">
        {view.phases.map((p, i) => {
          const next = view.phases[i + 1]
          const last = !next
          return (
            <li
              key={p.id}
              className={cn(
                'flex flex-col sm:flex-row sm:items-start',
                !last && 'sm:flex-1',
              )}
            >
              <UpgradeStageNode
                phase={p}
                index={i}
                selected={open?.id === p.id}
                motion={motion}
                panelId={panelId}
                onSelect={() => setPicked(p.id)}
              />
              {next ? (
                <span
                  aria-hidden
                  className={cn(
                    'ml-[1.4rem] h-5 w-0.5 rounded-full sm:ml-0 sm:mt-[1.4rem] sm:h-0.5 sm:w-auto sm:flex-1',
                    connectorClass(
                      p.state === 'ok' || p.state === 'skipped',
                      next.state === 'running',
                      motion,
                    ),
                  )}
                />
              ) : null}
            </li>
          )
        })}
      </ol>

      {open ? (
        <UpgradePhaseDetail
          id={panelId}
          phase={open}
          log={attempt?.log ?? []}
          error={attempt?.error ?? ''}
          now={now}
          motion={motion}
          percent={view.percent}
        />
      ) : null}

      {outcome === 'succeeded' && attempt ? (
        <div className="relative flex flex-wrap items-center gap-x-3 gap-y-1 rounded-md bg-success/10 p-3 text-sm">
          {motion ? <UpgradeSuccessBurst /> : null}
          <CircleCheckGlassIcon className="size-6 text-success" aria-hidden />
          <span className="font-mono">
            {attempt.from_version} {t('pipeline.to')} {attempt.to_version}
          </span>
          <span className="text-muted-foreground">
            {t('pipeline.totalTime', { time: formatElapsed(end - begin) })}
          </span>
          {attempt.backup_name ? (
            <span className="text-muted-foreground">
              {t('pipeline.backupAt', { name: attempt.backup_name })}
            </span>
          ) : null}
          {showReload ? (
            <button
              type="button"
              className="ml-auto rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground"
              onClick={() => window.location.reload()}
            >
              {t('pipeline.reload')}
            </button>
          ) : null}
        </div>
      ) : null}
      {outcome !== 'running' && outcome !== 'succeeded' && attempt ? (
        <div
          role="alert"
          className={cn(
            'flex items-start gap-2 rounded-md border p-3 text-sm',
            outcome === 'rolled_back'
              ? 'border-tone-warning-border bg-tone-warning-soft text-tone-warning'
              : 'border-destructive/40 bg-destructive/10 text-destructive',
          )}
        >
          <TriangleAlertGlassIcon
            className="mt-0.5 size-6 shrink-0 text-tone-warning"
            aria-hidden
          />
          <div className="space-y-1">
            {outcome === 'rolled_back' ? (
              <p className="font-medium">
                {t('pipeline.rolledBack', { version: attempt.from_version })}
              </p>
            ) : (
              <p className="font-medium">
                {t('progress.untouched', { version: attempt.from_version })}
              </p>
            )}
            {attempt.failed_step ? (
              <p>
                {t('progress.failedAt', {
                  step: t(`selfUpgrade.step.${attempt.failed_step}`, {
                    defaultValue: attempt.failed_step,
                  }),
                })}
              </p>
            ) : null}
            <p>{t('progress.nextAction')}</p>
          </div>
        </div>
      ) : null}
    </section>
  )
}
