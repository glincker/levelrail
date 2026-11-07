import { useQuery } from '@tanstack/react-query'
import {
  CheckGlassIcon,
  CircleDashedGlassIcon,
  CircleMinusGlassIcon,
  CircleXGlassIcon,
  ClockGlassIcon,
} from '@thesvg/ui-react'
import { useNowTick } from '../../hooks/useNowTick'
import { formatDurationMs } from '../../lib/deployDuration'
import {
  deployPhasesQueryOptions,
  type DeployPhase,
  type DeployPhaseStatus,
} from '../../queries/deployPhases'
import { useDeployDetailT } from './useDeployDetailT'

// Quiet this long with a running deploy shows the calm "still working" note.
const QUIET_AFTER_MS =
  Number(import.meta.env.VITE_DEPLOY_QUIET_AFTER_SECONDS ?? 60) * 1000

const STATUS_TONE: Record<DeployPhaseStatus, string> = {
  pending: 'text-muted-foreground',
  running: 'text-tone-info',
  done: 'text-tone-success',
  failed: 'text-tone-danger',
  skipped: 'text-muted-foreground',
}

function StatusIcon({ status }: { status: DeployPhaseStatus }) {
  const cls = `size-4 shrink-0 ${STATUS_TONE[status]}`
  switch (status) {
    case 'done':
      return <CheckGlassIcon className={cls} aria-hidden />
    case 'failed':
      return <CircleXGlassIcon className={cls} aria-hidden />
    case 'running':
      return (
        <ClockGlassIcon
          className={`${cls} motion-safe:animate-pulse`}
          aria-hidden
        />
      )
    case 'skipped':
      return <CircleMinusGlassIcon className={cls} aria-hidden />
    default:
      return <CircleDashedGlassIcon className={cls} aria-hidden />
  }
}

function minutes(ms: number): number {
  return Math.max(1, Math.round(ms / 60_000))
}

/** DeployPhases is a compact stepper above the log: where the deploy is, how long, and what is left. */
export function DeployPhases({
  deployId,
  running,
  lastEventAt,
}: {
  deployId: string
  running: boolean
  lastEventAt: number | null
}) {
  const { t } = useDeployDetailT()
  const { data } = useQuery(deployPhasesQueryOptions(deployId, running))
  const now = useNowTick(running)
  if (!data || data.phases.length === 0) return null

  const quiet =
    running && lastEventAt !== null && now - lastEventAt > QUIET_AFTER_MS
  const exportRunning = data.phases.some(
    (p) => p.phase === 'export_image' && p.status === 'running',
  )

  const timeLabel = (p: DeployPhase): string | null => {
    if (p.duration_ms === null) return null
    const started = p.started_at ? Date.parse(p.started_at) : NaN
    const ms =
      p.status === 'running' && Number.isFinite(started)
        ? now - started
        : p.duration_ms
    return formatDurationMs(ms)
  }

  return (
    <section aria-label={t('deployPhases.label')} className="space-y-2">
      <ol className="flex flex-wrap items-center gap-2">
        {data.phases.map((p) => {
          const time = timeLabel(p)
          return (
            <li
              key={p.phase}
              aria-current={p.status === 'running' ? 'step' : undefined}
              className="flex items-center gap-1.5 rounded-full border border-border px-3 py-1 text-xs"
            >
              <StatusIcon status={p.status} />
              <span className="font-medium">
                {t(`deployPhases.phase.${p.phase}`)}
              </span>
              <span className="sr-only">
                {t(`deployPhases.status.${p.status}`)}
              </span>
              {time ? (
                <span className="text-muted-foreground tabular-nums">
                  {time}
                </span>
              ) : null}
            </li>
          )
        })}
      </ol>
      {running && data.eta_ms !== null && data.typical_total_ms !== null ? (
        <p className="text-xs text-muted-foreground">
          {data.eta_ms > 0
            ? t('deployPhases.eta', {
                typical: minutes(data.typical_total_ms),
                left: minutes(data.eta_ms),
              })
            : t('deployPhases.etaOver', {
                typical: minutes(data.typical_total_ms),
              })}
        </p>
      ) : null}
      {quiet ? (
        <p role="status" className="text-xs text-muted-foreground">
          {exportRunning
            ? t('deployPhases.stillWorkingExport')
            : t('deployPhases.stillWorking')}
        </p>
      ) : null}
    </section>
  )
}
