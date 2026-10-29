import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import { formatDimension } from '../../lib/placementFormat'
import { useNodeAllocations } from '../../queries/placementDecision'
import type { PlacementDimension } from '../../types/placementDecision'

function DimensionBar({ d }: { d: PlacementDimension }) {
  const { t } = useTranslation('nodes')
  const label = t(`allocation.dimension.${d.name}`)
  if (d.allocatable === null) {
    return (
      <div className="space-y-1">
        <p className="text-sm font-medium text-foreground">{label}</p>
        <p className="text-xs text-muted-foreground">
          {t('allocation.unknown')}
        </p>
      </div>
    )
  }
  const pct =
    d.allocatable > 0 ? Math.round((d.requested / d.allocatable) * 100) : 0
  return (
    <div className="space-y-1">
      <div className="flex items-baseline justify-between gap-2">
        <p className="text-sm font-medium text-foreground">{label}</p>
        <p
          className={
            pct > 100
              ? 'text-xs text-destructive tabular-nums'
              : 'text-xs text-muted-foreground tabular-nums'
          }
        >
          {t(
            d.name === 'disk' ? 'allocation.usedOf' : 'allocation.requestedOf',
            {
              used: formatDimension(d, d.requested),
              total: formatDimension(d, d.allocatable),
              pct,
            },
          )}
        </p>
      </div>
      <Progress
        value={Math.min(pct, 100)}
        aria-label={t('allocation.barLabel', { name: label, pct })}
      />
    </div>
  )
}

// Requested versus allocatable CPU, memory and disk for one node, from the
// fleet-wide GET /api/v1/nodes/allocations (one request, no per-node fetch).
export function NodeAllocationCard({ nodeId }: { nodeId: string }) {
  const { t } = useTranslation('nodes')
  const allocations = useNodeAllocations()
  const node = allocations.data?.nodes.find((n) => n.node_id === nodeId)

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('allocation.title')}</CardTitle>
        <CardDescription>{t('allocation.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {allocations.isPending ? (
          <Skeleton className="h-20 w-full" />
        ) : allocations.isError ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.7em"
            />
            <AlertDescription className="space-y-2">
              <p>
                {t('allocation.loadFailed')} {allocations.error.message}
              </p>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => void allocations.refetch()}
              >
                {t('placement.retry')}
              </Button>
            </AlertDescription>
          </Alert>
        ) : !node ? (
          <p className="text-sm text-muted-foreground">
            {t('allocation.empty')}
          </p>
        ) : (
          <>
            {node.dimensions.map((d) => (
              <DimensionBar key={d.name} d={d} />
            ))}
            <p className="text-xs text-muted-foreground">
              {t('allocation.policy', {
                cpu: allocations.data.policy.cpu_overcommit_percent,
                memory: allocations.data.policy.memory_overcommit_percent,
                disk: allocations.data.policy.min_free_disk_percent,
                workloads: node.workloads,
              })}
            </p>
          </>
        )}
      </CardContent>
    </Card>
  )
}
