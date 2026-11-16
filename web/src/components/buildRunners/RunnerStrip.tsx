import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { StatusBadge } from '@/components/ui/status-badge'
import { capacityPercent, runnerState } from '../../lib/buildRunnerFormat'
import type { BuildRunner, RunnerStatus } from '../../queries/buildRunners'

// One runner on one line: name, role, status, slots, load.
export function RunnerStrip({ runner }: Readonly<{ runner: BuildRunner }>) {
  const { t } = useTranslation('deploys')
  const statusLabels: Record<RunnerStatus, string> = {
    ready: t('runners.status.ready'),
    busy: t('runners.status.busy'),
    pressure: t('runners.status.pressure'),
    draining: t('runners.status.draining'),
    offline: t('runners.status.offline'),
  }
  return (
    <li
      data-testid="runner-strip-item"
      className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-sm"
    >
      <span className="font-medium text-foreground">{runner.name}</span>
      <Badge variant="muted">
        {t(
          runner.kind === 'control_plane'
            ? 'runners.kind.control_plane'
            : 'runners.kind.node',
        )}
      </Badge>
      <StatusBadge
        state={runnerState(runner.status)}
        form="dot"
        label={statusLabels[runner.status] ?? runner.status}
      />
      <span className="text-xs text-muted-foreground tabular-nums">
        {t('runners.slots', {
          used: runner.slots_used,
          total: runner.slots_total,
          percent: capacityPercent(runner.slots_used, runner.slots_total),
        })}
        {', '}
        {t('runners.load', { load: runner.load.toFixed(2) })}
      </span>
    </li>
  )
}
