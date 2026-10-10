import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { GaugeIcon } from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { SkeletonTile } from '@/components/kit'
import type { AppListEntry } from '../../types/appDetail'
import { useDatabases } from '../../queries/databases'
import { useFleetResourceUsage } from '../../queries/fleetResourceUsage'
import { useAppResourceUsage } from '../../queries/appResourceUsage'
import { useUsageSummary } from '../../queries/usageSummary'
import { useProjectListOptional } from '../../queries/projects'
import { formatCores, formatSize } from '../../lib/format'
import {
  groupReservations,
  summarizeReserved,
  topReservers,
  usageKey,
  type GroupBy,
  type Reservable,
  type UsageReading,
} from '../../lib/reservedUsage'
import { ReservedBreakdown } from './ReservedBreakdown'
import { ReservedResourceColumn } from './ReservedResourceColumn'

const TOP_RESERVERS = 5

export function ReservedUsageCard({ apps }: { apps: AppListEntry[] }) {
  const { t } = useTranslation('dashboard')
  const dbs = useDatabases()
  const fleet = useFleetResourceUsage()
  const appUsage = useAppResourceUsage()
  const summaryQ = useUsageSummary()
  const projects = useProjectListOptional()

  const model = useMemo(() => {
    const projectName = new Map(
      (projects.data ?? []).map((p) => [p.id, p.name]),
    )
    const none = t('fleet.reserved.noProject')
    const noEnv = t('fleet.reserved.noEnvironment')
    const resources: Reservable[] = [
      ...apps.map((a): Reservable => ({
        kind: 'app',
        name: a.name,
        projectId: a.project_id ?? '',
        environment: a.environment_name ?? '',
        replicas: a.replicas,
        suspended: a.suspended,
        resources: a.resources,
      })),
      ...(dbs.data ?? []).map((d): Reservable => ({
        kind: 'database',
        name: d.name,
        projectId: d.project_id ?? '',
        environment: '',
        replicas: 1,
        suspended: d.suspended ?? false,
        resources: d.resources,
      })),
    ]
    const usage = new Map<string, UsageReading>()
    for (const u of appUsage.data ?? []) {
      usage.set(usageKey('app', u.name), {
        cpuPercent: u.cpu_percent,
        memoryBytes: u.memory_usage_bytes,
      })
    }
    for (const u of summaryQ.data?.databases ?? []) {
      usage.set(usageKey('database', u.name), {
        cpuPercent: u.cpu_percent,
        memoryBytes: u.memory_usage_bytes,
      })
    }
    const f = fleet.data?.fleet
    const nodeCount = f?.node_count ?? 0
    const summary = summarizeReserved(
      resources,
      usage,
      {
        cpuCores: summaryQ.data?.local_cpu_cores,
        memoryBytes: f?.total_memory_bytes,
        cpuComplete: nodeCount === 1,
        memoryComplete:
          f !== undefined &&
          nodeCount > 0 &&
          f.nodes_with_memory_capacity >= nodeCount,
      },
      appUsage.isSuccess && summaryQ.isSuccess,
    )
    const labelFor = (r: Reservable) =>
      r.kind === 'database' && !r.environment
        ? noEnv
        : (projectName.get(r.projectId) ?? none)
    return {
      summary,
      nodeCount,
      memoryKnown: f?.nodes_with_memory_capacity ?? 0,
      rowsFor: (by: GroupBy) =>
        groupReservations(resources, summary, by, (r) =>
          by === 'project' ? labelFor(r) : noEnv,
        ),
    }
  }, [apps, dbs.data, fleet.data, appUsage, summaryQ, projects.data, t])

  if (dbs.isPending || summaryQ.isPending) {
    return <SkeletonTile className="h-56" />
  }
  const { summary } = model
  if (summary.active === 0 && summary.suspended === 0) return null

  const memHint =
    model.nodeCount > 0 && model.memoryKnown < model.nodeCount
      ? t('fleet.reserved.capacityPartial', {
          known: model.memoryKnown,
          total: model.nodeCount,
        })
      : undefined

  return (
    <Card size="sm">
      <CardHeader className="space-y-0">
        <CardTitle className="flex items-center gap-1.5 text-sm font-medium">
          <GaugeIcon
            className="size-4 text-muted-foreground"
            aria-hidden="true"
          />
          {t('fleet.reserved.title')}
        </CardTitle>
        <p className="text-xs text-muted-foreground">
          {t('fleet.reserved.subtitle', {
            active: summary.active,
            suspended: summary.suspended,
          })}
        </p>
      </CardHeader>
      <CardContent className="space-y-6">
        <div className="grid gap-6 sm:grid-cols-2">
          <ReservedResourceColumn
            label={t('fleet.reserved.cpu')}
            totals={summary.cpu}
            format={(n) => `${formatCores(n)} ${t('fleet.reserved.cores')}`}
            capacityHint={t('fleet.reserved.cpuCapacityScope')}
            noLimitKey="cpu"
          />
          <ReservedResourceColumn
            label={t('fleet.reserved.memory')}
            totals={summary.memory}
            format={formatSize}
            capacityHint={memHint}
            noLimitKey="memory"
          />
        </div>
        <ReservedBreakdown
          rowsFor={model.rowsFor}
          top={topReservers(summary, TOP_RESERVERS)}
        />
      </CardContent>
    </Card>
  )
}
