import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  CircleQuestionMarkGlassIcon,
  CircleXGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { formatDate } from '../../lib/format'
import { formatDimension } from '../../lib/placementFormat'
import { useNodePools, useSetAppPool } from '../../queries/nodePools'
import { usePlacementDecision } from '../../queries/placementDecision'
import type { NodeMoveKind } from '../../types/nodeMoves'
import type {
  PlacementDecision,
  PlacementEvaluation,
} from '../../types/placementDecision'
import { useNodeNames } from './useNodeNames'
import { VirtualRows } from './VirtualRows'

function EvaluationRow({ e }: { e: PlacementEvaluation }) {
  const { t } = useTranslation('nodes')
  const nodeName = useNodeNames()
  const state = !e.eligible ? 'refused' : e.unverified ? 'unverified' : 'fits'
  const Icon =
    state === 'refused'
      ? CircleXGlassIcon
      : state === 'unverified'
        ? CircleQuestionMarkGlassIcon
        : CircleCheckGlassIcon
  const detail = e.eligible
    ? e.dimensions
        .map((d) =>
          d.allocatable === null || d.free === null
            ? t('decision.dimensionUnknown', {
                name: t(`allocation.dimension.${d.name}`),
              })
            : t('decision.dimensionFree', {
                name: t(`allocation.dimension.${d.name}`),
                free: formatDimension(d, Math.max(d.free, 0)),
              }),
        )
        .join(', ')
    : e.reason
  return (
    <div className="flex items-start gap-2 px-3 py-2 text-sm">
      <Icon className="mt-0.5 size-5 shrink-0" aria-hidden="true" />
      <div className="min-w-0 space-y-0.5">
        <p className="font-medium text-foreground">
          {nodeName(e.node_id)}{' '}
          <span className="font-normal text-muted-foreground">
            {t(`decision.state.${state}`)}
          </span>
        </p>
        <p className="text-xs break-words text-muted-foreground">{detail}</p>
      </div>
    </div>
  )
}

function DecisionBlock({
  title,
  decision,
}: {
  title: string
  decision: PlacementDecision
}) {
  const { t } = useTranslation('nodes')
  const nodeName = useNodeNames()
  const considered = decision.considered ?? []
  return (
    <div className="space-y-2">
      <p className="text-sm font-medium text-foreground">{title}</p>
      {decision.node_id || decision.why ? (
        <p className="text-sm text-muted-foreground">
          {t('decision.winner', { node: nodeName(decision.node_id) })}{' '}
          {decision.why}
        </p>
      ) : (
        <p className="text-sm text-destructive">{t('decision.nothingFits')}</p>
      )}
      {(decision.warnings ?? []).map((w) => (
        <Alert key={w}>
          <TriangleAlertGlassIcon className="text-tone-warning" size="1.7em" />
          <AlertDescription>{w}</AlertDescription>
        </Alert>
      ))}
      {considered.length > 0 ? (
        <VirtualRows
          items={considered}
          rowKey={(e) => e.node_id}
          renderRow={(e) => <EvaluationRow e={e} />}
          estimateSize={56}
        />
      ) : null}
    </div>
  )
}

const NO_POOL = '__none__'

// Targets an app at a named pool instead of repeating selectors per app.
// Hidden when no pool exists or the control plane has no multi-node support.
function PoolPicker({ app }: { app: string }) {
  const { t } = useTranslation('nodes')
  const pools = useNodePools()
  const setPool = useSetAppPool(app)
  if (!pools.data || pools.data.length === 0) return null
  return (
    <div className="space-y-1">
      <label
        htmlFor={`pool-${app}`}
        className="text-sm font-medium text-foreground"
      >
        {t('pools.pickerLabel')}
      </label>
      <Select
        value={NO_POOL}
        onValueChange={(v) => {
          if (v !== null) setPool.mutate(v === NO_POOL ? '' : v)
        }}
      >
        <SelectTrigger id={`pool-${app}`} className="w-full sm:w-64">
          <SelectValue placeholder={t('pools.pickerPlaceholder')} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={NO_POOL}>{t('pools.none')}</SelectItem>
          {pools.data.map((p) => (
            <SelectItem key={p.name} value={p.name}>
              {p.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p className="text-xs text-muted-foreground">{t('pools.pickerHint')}</p>
      {setPool.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {setPool.error.message}
        </p>
      ) : null}
    </div>
  )
}

// Why an app or database runs where it does: the recorded decision (which
// nodes were considered, which were refused and why, the winner) and a
// fresh evaluation of every node right now.
export function PlacementDecisionPanel({
  kind,
  name,
}: {
  kind: NodeMoveKind
  name: string
}) {
  const { t } = useTranslation('nodes')
  const decision = usePlacementDecision(kind, name)

  if (decision.isPending) {
    return <Skeleton className="h-20 w-full" />
  }
  if (decision.isError) {
    return (
      <Alert variant="destructive">
        <TriangleAlertGlassIcon className="text-tone-warning" size="1.7em" />
        <AlertDescription className="space-y-2">
          <p>
            {t('decision.loadFailed')} {decision.error.message}
          </p>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => void decision.refetch()}
          >
            {t('placement.retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  }
  const { recorded, preview } = decision.data
  return (
    <div className="space-y-4">
      {kind === 'app' ? <PoolPicker app={name} /> : null}
      {recorded ? (
        <div className="space-y-2">
          <Badge variant="outline">
            {t(`decision.trigger.${recorded.trigger}`)}{' '}
            {formatDate(recorded.decided_at, '')}
          </Badge>
          <DecisionBlock
            title={t('decision.recordedTitle')}
            decision={recorded.decision}
          />
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">
          {t('decision.noneRecorded')}
        </p>
      )}
      <DecisionBlock title={t('decision.previewTitle')} decision={preview} />
    </div>
  )
}
