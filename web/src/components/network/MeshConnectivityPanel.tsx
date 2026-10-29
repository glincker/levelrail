import { useTranslation } from 'react-i18next'
import { RefreshCwGlassIcon, WifiOffGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { ApiError } from '../../lib/apiError'
import { useMeshConnectivity } from '../../queries/nodeMoves'
import type {
  MeshConnectivityPair,
  MeshLinkView,
  MeshPairStatus,
} from '../../types/nodeMoves'
import { VirtualRows } from '../nodes/VirtualRows'

const PAIR_VARIANT: Record<
  MeshPairStatus,
  'success' | 'warning' | 'destructive' | 'muted'
> = {
  ok: 'success',
  degraded: 'warning',
  down: 'destructive',
  unknown: 'muted',
}

// Pair matrix from GET /api/v1/mesh/connectivity: each node's WireGuard
// view of every other node, with the server's hint for a bad pair.
export function MeshConnectivityPanel() {
  const { t } = useTranslation('nodes')
  const query = useMeshConnectivity()

  const meshOff = query.error instanceof ApiError && query.error.status === 501

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('connectivity.title')}</CardTitle>
        <CardDescription>{t('connectivity.description')}</CardDescription>
        {meshOff ? null : (
          <CardAction>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={query.isFetching}
              onClick={() => void query.refetch()}
            >
              <RefreshCwGlassIcon size="1.2em" />
              {t('connectivity.refresh')}
            </Button>
          </CardAction>
        )}
      </CardHeader>
      <CardContent className="space-y-3">
        {query.isPending ? (
          <Skeleton className="h-16 w-full" />
        ) : meshOff ? (
          <EmptyState
            icon={<WifiOffGlassIcon className="size-5.5" />}
            title={t('connectivity.off')}
            description={t('connectivity.offDescription')}
          />
        ) : query.isError ? (
          <Alert variant="destructive">
            <AlertDescription className="space-y-2">
              <p>
                {t('connectivity.loadFailed')} {query.error.message}
              </p>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => void query.refetch()}
              >
                {t('connectivity.retry')}
              </Button>
            </AlertDescription>
          </Alert>
        ) : query.data.pairs.length === 0 ? (
          <EmptyState
            icon={<WifiOffGlassIcon className="size-5.5" />}
            title={t('connectivity.empty')}
            description={t('connectivity.emptyDescription')}
          />
        ) : (
          <Matrix data={query.data} />
        )}
      </CardContent>
    </Card>
  )
}

function Matrix({
  data,
}: {
  data: NonNullable<ReturnType<typeof useMeshConnectivity>['data']>
}) {
  const { t } = useTranslation('nodes')
  const names = new Map(data.nodes.map((n) => [n.id, n.name || n.id]))
  const nameOf = (id: string) => names.get(id) ?? id

  function link(from: string, to: string, view: MeshLinkView) {
    const handshake = view.last_handshake
      ? new Date(view.last_handshake).toLocaleString()
      : t('connectivity.never')
    return (
      <p className="text-xs text-muted-foreground">
        <span className="text-foreground">
          {t('connectivity.direction', { from, to })}
        </span>
        : {t(`connectivity.linkState.${view.state}`)}, {handshake}
        {view.endpoint ? (
          <span className="font-mono"> {view.endpoint}</span>
        ) : null}
      </p>
    )
  }

  function row(pair: MeshConnectivityPair) {
    const a = nameOf(pair.a)
    const b = nameOf(pair.b)
    return (
      <div className="space-y-1 px-3 py-2">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <p className="text-sm font-medium text-foreground">
            {a} <span className="text-muted-foreground">/</span> {b}
          </p>
          <Badge variant={PAIR_VARIANT[pair.status]}>
            {t(`connectivity.pairStatus.${pair.status}`)}
          </Badge>
        </div>
        {link(a, b, pair.a_to_b)}
        {link(b, a, pair.b_to_a)}
        {pair.hint ? (
          <p className="text-xs text-foreground">{pair.hint}</p>
        ) : null}
      </div>
    )
  }

  const viewErrors = data.nodes.filter((n) => n.view_error)

  return (
    <>
      {viewErrors.map((n) => (
        <Alert key={n.id} variant="destructive">
          <AlertDescription>
            {t('connectivity.viewError', {
              node: n.name || n.id,
              error: n.view_error,
            })}
          </AlertDescription>
        </Alert>
      ))}
      <VirtualRows
        items={data.pairs}
        rowKey={(p) => `${p.a}:${p.b}`}
        estimateSize={92}
        renderRow={row}
      />
      <p className="text-xs text-muted-foreground">
        {t('connectivity.checkedAt', {
          time: new Date(data.checked_at).toLocaleTimeString(),
        })}
      </p>
    </>
  )
}
