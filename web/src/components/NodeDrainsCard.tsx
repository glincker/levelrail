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
import { Skeleton } from '@/components/ui/skeleton'
import { useNodeDrains } from '../queries/nodeMoves'
import { DrainStatusBadge, NodeDrainProgress } from './nodes/NodeDrainProgress'
import { useNodeNames } from './nodes/useNodeNames'

// Recent drains of one node, live while any of them is running.
export function NodeDrainsCard({ nodeId }: { nodeId: string }) {
  const { t } = useTranslation('nodes')
  const nodeName = useNodeNames()
  const drains = useNodeDrains(nodeId)

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('drains.title')}</CardTitle>
        <CardDescription>{t('drains.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {drains.isPending ? (
          <Skeleton className="h-16 w-full" />
        ) : drains.isError ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.4em"
            />
            <AlertDescription className="space-y-2">
              <p>
                {t('drains.loadFailed')} {drains.error.message}
              </p>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => void drains.refetch()}
              >
                {t('drains.retry')}
              </Button>
            </AlertDescription>
          </Alert>
        ) : drains.data.length === 0 ? (
          <div className="text-sm">
            <p className="text-foreground">{t('drains.empty')}</p>
            <p className="text-muted-foreground">{t('drains.emptyHint')}</p>
          </div>
        ) : (
          <div className="divide-y divide-border">
            {drains.data.map((drain) => (
              <div
                key={drain.id}
                className="space-y-2 py-3 first:pt-0 last:pb-0"
              >
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="text-sm">
                    <p className="font-medium text-foreground">
                      {t('drains.startedAt', {
                        time: new Date(drain.started_at).toLocaleString(),
                      })}
                    </p>
                    <p className="text-xs text-muted-foreground">
                      {t('drains.moveCount', { count: drain.moves.length })}
                      {drain.target_node_id
                        ? `, ${t('drain.toNode', { node: nodeName(drain.target_node_id) })}`
                        : `, ${t('drain.toAuto')}`}
                    </p>
                  </div>
                  <DrainStatusBadge status={drain.status} />
                </div>
                <NodeDrainProgress drain={drain} />
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
