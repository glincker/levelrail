import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import type { NodeDrain, NodeDrainStatus } from '../../types/nodeMoves'
import { MoveStatusBadge } from './NodeMoveSteps'
import { useNodeNames } from './useNodeNames'
import { VirtualRows } from './VirtualRows'

const DRAIN_VARIANT: Record<
  NodeDrainStatus,
  'muted' | 'success' | 'destructive'
> = {
  running: 'muted',
  succeeded: 'success',
  failed: 'destructive',
}

export function DrainStatusBadge({ status }: { status: NodeDrainStatus }) {
  const { t } = useTranslation('nodes')
  return (
    <Badge variant={DRAIN_VARIANT[status]}>{t(`drain.status.${status}`)}</Badge>
  )
}

// Body of one drain: error, blocked items and every move with its phase.
export function NodeDrainProgress({ drain }: { drain: NodeDrain }) {
  const { t } = useTranslation('nodes')
  const nodeName = useNodeNames()
  const nothingToMove =
    drain.moves.length === 0 &&
    drain.blocked.length === 0 &&
    drain.status === 'succeeded'

  return (
    <div className="space-y-3">
      {drain.error ? (
        <Alert variant="destructive">
          <TriangleAlertGlassIcon className="text-tone-warning" size="1.2em" />
          <AlertTitle>{t('drain.errorTitle')}</AlertTitle>
          <AlertDescription>
            <p>{drain.error}</p>
            <p>{t('drain.failedHint')}</p>
          </AlertDescription>
        </Alert>
      ) : null}

      {nothingToMove ? (
        <p className="text-sm text-muted-foreground">{t('drain.emptyDone')}</p>
      ) : null}

      {drain.blocked.length > 0 ? (
        <div>
          <p className="mb-1 text-xs font-medium tracking-wide text-muted-foreground uppercase">
            {t('drain.blockedTitle')}
          </p>
          <ul className="space-y-0.5 text-sm">
            {drain.blocked.map((b) => (
              <li key={`${b.kind}-${b.name}`} className="text-xs">
                <span className="font-mono">
                  {b.kind} {b.name}
                </span>
                <span className="text-muted-foreground">: {b.reason}</span>
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      {drain.moves.length > 0 ? (
        <div>
          <p className="mb-1 text-xs font-medium tracking-wide text-muted-foreground uppercase">
            {t('drain.movesTitle')}
          </p>
          <VirtualRows
            items={drain.moves}
            rowKey={(m) => m.id}
            estimateSize={56}
            renderRow={(m) => (
              <div className="flex flex-wrap items-center justify-between gap-2 px-3 py-2">
                <div className="min-w-0">
                  <p className="truncate font-mono text-xs text-foreground">
                    {m.kind} {m.resource}
                  </p>
                  <p className="truncate text-xs text-muted-foreground">
                    {nodeName(m.from_node_id)} {'>'} {nodeName(m.to_node_id)}
                    {m.status === 'running' ? `, ${t(`phase.${m.phase}`)}` : ''}
                  </p>
                  {m.error ? (
                    <p className="text-xs break-words text-destructive">
                      {m.error}
                    </p>
                  ) : null}
                </div>
                <MoveStatusBadge status={m.status} />
              </div>
            )}
          />
        </div>
      ) : null}
    </div>
  )
}
