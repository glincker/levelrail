import { useTranslation } from 'react-i18next'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Progress } from '@/components/ui/progress'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { formatBytes } from '../../lib/format'
import { useNodeCapacity, useNodeCleanupRuns } from '../../queries/nodeCapacity'
import type { CapacityLevel, CapacityMetric } from '../../types/nodeCapacity'
import { METRIC_LABEL } from './capacityKeys'
import { FreeSpaceDialog } from './FreeSpaceDialog'
import { NodeGuardrailSettings } from './NodeGuardrailSettings'

const LEVEL_BAR: Record<CapacityLevel, string> = {
  unknown: '',
  ok: '',
  warn: '[&_[data-slot=progress-indicator]]:bg-warning',
  critical: '[&_[data-slot=progress-indicator]]:bg-destructive',
}

function MetricBar({ metric }: Readonly<{ metric: CapacityMetric }>) {
  const { t } = useTranslation('nodes')
  const used = metric.name === 'swap'
  const filled = used ? metric.percent : 100 - metric.percent
  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between text-sm">
        <span className="font-medium">{t(METRIC_LABEL[metric.name])}</span>
        {metric.level === 'unknown' ? (
          <span className="text-muted-foreground">
            {t('capacity.noReading')}
          </span>
        ) : (
          <span className="flex items-center gap-2">
            {metric.level === 'ok' ? null : (
              <Badge
                variant={
                  metric.level === 'critical' ? 'destructive' : 'warning'
                }
              >
                {metric.level === 'critical'
                  ? t('capacity.levels.critical')
                  : t('capacity.levels.warn')}
              </Badge>
            )}
            {used
              ? t('capacity.usedPct', { pct: metric.percent.toFixed(1) })
              : t('capacity.freePct', { pct: metric.percent.toFixed(1) })}
            {metric.name === 'inodes' ? null : (
              <span className="text-muted-foreground">
                {formatBytes(metric.free)} / {formatBytes(metric.total)}
              </span>
            )}
          </span>
        )}
      </div>
      <Progress
        value={metric.level === 'unknown' ? 0 : filled}
        className={LEVEL_BAR[metric.level]}
        aria-label={t(METRIC_LABEL[metric.name])}
      />
    </div>
  )
}

function History({ nodeId }: Readonly<{ nodeId: string }>) {
  const { t } = useTranslation('nodes')
  const { data } = useNodeCleanupRuns(nodeId)
  if (!data || data.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">{t('capacity.noRuns')}</p>
    )
  }
  return (
    <ul className="space-y-1 text-sm">
      {data.map((r) => (
        <li key={r.id} className="flex justify-between gap-2">
          <span>
            {new Date(r.started_at).toLocaleString()} (
            {r.trigger === 'auto'
              ? t('capacity.triggers.auto')
              : t('capacity.triggers.manual')}
            )
          </span>
          <span>
            {t('capacity.freed', { size: formatBytes(r.freed_bytes) })}
          </span>
        </li>
      ))}
    </ul>
  )
}

export function NodeCapacityCard({ nodeId }: Readonly<{ nodeId: string }>) {
  const { t } = useTranslation('nodes')
  const { data, isPending, isError } = useNodeCapacity(nodeId)

  if (isPending || isError || !data) {
    return null
  }

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-2">
        <CardTitle>{t('capacity.title')}</CardTitle>
        {data.cleanup_supported ? <FreeSpaceDialog nodeId={nodeId} /> : null}
      </CardHeader>
      <CardContent className="space-y-4">
        {data.status.worst === 'critical' ? (
          <Alert variant="destructive">
            <AlertDescription>
              {t('capacity.blocked', { reason: data.status.reason })}
            </AlertDescription>
          </Alert>
        ) : null}
        {data.has_reading ? (
          data.status.metrics.map((m) => <MetricBar key={m.name} metric={m} />)
        ) : (
          <p className="text-sm text-muted-foreground">
            {t('capacity.waiting')}
          </p>
        )}
        {data.cleanup_supported ? null : (
          <p className="text-xs text-muted-foreground">
            {t('capacity.cleanupUnsupported')}
          </p>
        )}
        <Collapsible>
          <CollapsibleTrigger className="text-sm font-medium">
            {t('capacity.settings')}
          </CollapsibleTrigger>
          <CollapsiblePanel className="pt-3">
            <NodeGuardrailSettings
              key={JSON.stringify(data.settings)}
              nodeId={nodeId}
              capacity={data}
            />
          </CollapsiblePanel>
        </Collapsible>
        <Collapsible>
          <CollapsibleTrigger className="text-sm font-medium">
            {t('capacity.history')}
          </CollapsibleTrigger>
          <CollapsiblePanel className="pt-3">
            <History nodeId={nodeId} />
          </CollapsiblePanel>
        </Collapsible>
      </CardContent>
    </Card>
  )
}
