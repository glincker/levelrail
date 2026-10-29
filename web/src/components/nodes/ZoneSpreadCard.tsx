import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
import { useZones } from '../../queries/placementDecision'
import { PlacementWhatIf } from './PlacementWhatIf'

const CONCENTRATION_PCT = 70

export function ZoneSpreadCard() {
  const { t } = useTranslation('nodes')
  const zones = useZones()

  if (zones.isPending) {
    return <Skeleton className="h-28 w-full" />
  }
  if (zones.isError) {
    return (
      <Alert variant="destructive">
        <TriangleAlertGlassIcon className="text-tone-warning" size="1.7em" />
        <AlertDescription className="space-y-2">
          <p>
            {t('zones.loadFailed')} {zones.error.message}
          </p>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => void zones.refetch()}
          >
            {t('zones.retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  }

  const { data } = zones
  if (data.nodes.length < 2) {
    return null
  }
  const total = data.zones.reduce((sum, z) => sum + z.workloads, 0)
  const top = data.zones.reduce((a, b) => (b.workloads > a.workloads ? b : a))
  const topPct = total > 0 ? Math.round((top.workloads / total) * 100) : 0
  const firstNode = data.nodes[0]

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('zones.title')}</CardTitle>
        <CardDescription>{t('zones.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {!data.labeled && firstNode ? (
          <Alert>
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.7em"
            />
            <AlertTitle>{t('zones.unlabeledTitle')}</AlertTitle>
            <AlertDescription className="space-y-2">
              <p>{t('zones.unlabeledBody')}</p>
              <Button
                size="sm"
                nativeButton={false}
                render={
                  <Link to="/nodes/$id" params={{ id: firstNode.node_id }} />
                }
              >
                {t('zones.fix')}
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}
        {data.zones.map((z) => {
          const pct = total > 0 ? Math.round((z.workloads / total) * 100) : 0
          const name = z.zone || t('zones.unlabeledZone')
          return (
            <div key={z.zone} className="space-y-1">
              <div className="flex items-baseline justify-between gap-2">
                <p className="text-sm font-medium text-foreground">
                  {name}
                  {z.region || z.providers?.length ? (
                    <span className="ml-2 text-xs font-normal text-muted-foreground">
                      {[...(z.providers ?? []), z.region]
                        .filter(Boolean)
                        .join(' / ')}
                    </span>
                  ) : null}
                </p>
                <p className="text-xs text-muted-foreground tabular-nums">
                  {t('zones.nodes', {
                    online: z.online_nodes,
                    total: z.node_ids.length,
                  })}
                  {', '}
                  {t('zones.workloads', { count: z.workloads })}
                </p>
              </div>
              <Progress
                value={pct}
                aria-label={t('zones.share', { pct, zone: name })}
              />
            </div>
          )
        })}
        {data.labeled &&
        data.zones.length > 1 &&
        topPct >= CONCENTRATION_PCT ? (
          <p className="text-xs text-destructive">{t('zones.concentrated')}</p>
        ) : null}
        <PlacementWhatIf zones={data.zones} />
      </CardContent>
    </Card>
  )
}
