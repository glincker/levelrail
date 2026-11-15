import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { CpuGlassIcon, ServerGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { StatusBadge } from '@/components/ui/status-badge'
import { RelativeTime } from '@/components/kit'
import { ObjectCard } from '@/components/cards/ObjectCard'
import { formatBytes } from '../../lib/format'
import { capacityPercent, runnerState } from '../../lib/buildRunnerFormat'
import type { BuildRunner, RunnerStatus } from '../../queries/buildRunners'
import { CapacityBar } from './CapacityBar'

export function RunnerCard({ runner }: Readonly<{ runner: BuildRunner }>) {
  const { t } = useTranslation('deploys')
  const isControlPlane = runner.kind === 'control_plane'
  const labels = runner.labels ?? []
  const status = runnerState(runner.status)
  const percent = capacityPercent(runner.slots_used, runner.slots_total)
  const statusLabels: Record<RunnerStatus, string> = {
    ready: t('runners.status.ready'),
    busy: t('runners.status.busy'),
    pressure: t('runners.status.pressure'),
    draining: t('runners.status.draining'),
    offline: t('runners.status.offline'),
  }
  const statusLabel = statusLabels[runner.status] ?? runner.status
  return (
    <ObjectCard
      data-testid="runner-card"
      icon={isControlPlane ? <CpuGlassIcon /> : <ServerGlassIcon />}
      title={runner.name}
      titleAs="h3"
      badges={
        <Badge variant="muted">
          {t(
            isControlPlane ? 'runners.kind.control_plane' : 'runners.kind.node',
          )}
        </Badge>
      }
      status={<StatusBadge state={status} label={statusLabel} />}
      meta={[
        runner.architecture,
        runner.cpu_cores
          ? t('runners.cores', { count: runner.cpu_cores })
          : null,
        t('runners.load', { load: runner.load.toFixed(2) }),
      ]}
    >
      <div className="space-y-1.5">
        <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
          <span>
            {t('runners.slots', {
              used: runner.slots_used,
              total: runner.slots_total,
              percent,
            })}
          </span>
          <span className="tabular-nums">
            {t('runners.waiting', { count: runner.waiting })}
          </span>
        </div>
        <CapacityBar
          used={runner.slots_used}
          total={runner.slots_total}
          label={t('runners.capacityLabel', { name: runner.name })}
        />
      </div>
      {labels.length > 0 ? (
        <ul aria-label={t('runners.labels')} className="flex flex-wrap gap-1.5">
          {labels.map((l) => (
            <li key={l}>
              <Badge variant="outline" className="font-mono">
                {l}
              </Badge>
            </li>
          ))}
        </ul>
      ) : null}
      <p className="min-w-0 text-xs text-muted-foreground">
        {runner.last_build_at ? (
          <>
            {t('runners.lastBuild')}{' '}
            {runner.last_build_app ? (
              <Link
                to="/apps/$name/deploys"
                params={{ name: runner.last_build_app }}
                className="font-medium text-foreground hover:underline"
              >
                {runner.last_build_app}
              </Link>
            ) : null}
            {runner.last_build_status ? `, ${runner.last_build_status}` : ''}
            {', '}
            <RelativeTime at={runner.last_build_at} />
          </>
        ) : (
          t('runners.noBuilds')
        )}
      </p>
      {isControlPlane && runner.cache_bytes !== undefined ? (
        <p className="text-xs text-muted-foreground">
          {t('runners.cache', { size: formatBytes(runner.cache_bytes) })}
        </p>
      ) : null}
    </ObjectCard>
  )
}
