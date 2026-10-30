import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
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
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { usePlacement, useSetPlacement } from '../queries/nodeMoves'
import {
  labelsFromRows,
  rowsFromLabels,
  sameLabels,
  type LabelRow,
} from '../lib/labelRows'
import type { NodeMoveKind, Placement } from '../types/nodeMoves'
import { MoveToNodeDialog } from './MoveToNodeDialog'
import { LabelRowsEditor } from './nodes/LabelRowsEditor'
import { PlacementDecisionPanel } from './nodes/PlacementDecisionPanel'
import { useNodeNames } from './nodes/useNodeNames'
import { MoveToThisServerButton, PlacementLabel } from './PlacementLabel'
import { useNodeListOptional } from '../queries/nodes'

// Placement of one app or database: where it runs, which label selector
// restricts it, which nodes are eligible, and the verified move action.
export function PlacementCard({
  kind,
  name,
  currentNodeId,
  currentNodeName,
  currentNodeIsLocal,
  volumeNames = [],
}: {
  kind: NodeMoveKind
  name: string
  currentNodeId?: string
  currentNodeName?: string
  currentNodeIsLocal?: boolean
  volumeNames?: string[]
}) {
  const { t } = useTranslation('nodes')
  const { data: nodeList } = useNodeListOptional()
  const placement = usePlacement(kind, name)
  const nodeId = placement.data?.node_id ?? currentNodeId ?? ''

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('placement.title')}</CardTitle>
        <CardDescription>
          {t('placement.currentNode')}:{' '}
          <PlacementLabel
            nodeId={nodeId}
            nodeName={nodeId === (currentNodeId ?? '') ? currentNodeName : ''}
            nodeIsLocal={
              nodeId === (currentNodeId ?? '') ? currentNodeIsLocal : false
            }
            nodes={nodeList}
          />
          <MoveToThisServerButton
            kind={kind}
            name={name}
            nodeId={nodeId}
            nodes={nodeList}
          />
        </CardDescription>
        <CardAction>
          <MoveToNodeDialog
            kind={kind}
            name={name}
            currentNodeId={nodeId}
            volumeNames={volumeNames}
          />
        </CardAction>
      </CardHeader>
      <CardContent className="space-y-4">
        {placement.isPending ? (
          <Skeleton className="h-20 w-full" />
        ) : placement.isError ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.7em"
            />
            <AlertDescription className="space-y-2">
              <p>
                {t('placement.loadFailed')} {placement.error.message}
              </p>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => void placement.refetch()}
              >
                {t('placement.retry')}
              </Button>
            </AlertDescription>
          </Alert>
        ) : (
          <PlacementBody
            key={JSON.stringify(placement.data.node_selector)}
            kind={kind}
            name={name}
            placement={placement.data}
          />
        )}
        {placement.data?.domain_warnings?.length ? (
          <Alert>
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.7em"
            />
            <AlertDescription className="space-y-1">
              <p className="font-medium">{t('placement.domainWarnings')}</p>
              <ul className="list-disc space-y-0.5 pl-4">
                {placement.data.domain_warnings.map((w) => (
                  <li key={w.kind}>{w.message}</li>
                ))}
              </ul>
            </AlertDescription>
          </Alert>
        ) : null}
        <PlacementDecisionPanel kind={kind} name={name} />
      </CardContent>
    </Card>
  )
}

function PlacementBody({
  kind,
  name,
  placement,
}: {
  kind: NodeMoveKind
  name: string
  placement: Placement
}) {
  const { t } = useTranslation('nodes')
  const nodeName = useNodeNames()
  const [rows, setRows] = useState<LabelRow[]>(() =>
    rowsFromLabels(placement.node_selector),
  )
  const save = useSetPlacement(kind, name)
  const parsed = labelsFromRows(rows)
  const dirty = !sameLabels(parsed.labels, placement.node_selector)

  return (
    <>
      <div className="space-y-2">
        <p className="text-sm font-medium text-foreground">
          {t('placement.selectorTitle')}
        </p>
        <p className="text-xs text-muted-foreground">
          {rows.length === 0
            ? t('placement.selectorEmpty')
            : t('placement.selectorHint')}
        </p>
        <LabelRowsEditor
          rows={rows}
          onChange={setRows}
          keyPlaceholder={t('placement.keyPlaceholder')}
          valuePlaceholder={t('placement.valuePlaceholder')}
          addLabel={t('placement.addRow')}
          removeLabel={(key) => t('placement.removeRow', { key })}
          disabled={save.isPending}
        />
        {parsed.error ? (
          <p className="text-sm text-destructive">
            {parsed.error === 'duplicate'
              ? t('placement.duplicate')
              : t('placement.incomplete')}
          </p>
        ) : null}
        {save.isError ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.7em"
            />
            <AlertDescription>{save.error.message}</AlertDescription>
          </Alert>
        ) : null}
        <Button
          type="button"
          size="sm"
          disabled={save.isPending || !dirty || parsed.error !== null}
          onClick={() =>
            save.mutate(parsed.labels, {
              onSuccess: () =>
                toast.add({ title: t('placement.saved'), type: 'success' }),
            })
          }
        >
          {save.isPending ? t('placement.saving') : t('placement.save')}
        </Button>
      </div>

      <div className="space-y-1.5">
        <p className="text-sm font-medium text-foreground">
          {t('placement.eligibleTitle')}
        </p>
        {placement.eligible_node_ids.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('placement.eligibleEmpty')}
          </p>
        ) : (
          <div className="flex flex-wrap gap-1.5">
            {placement.eligible_node_ids.map((id) => (
              <Badge key={id} variant="success">
                {nodeName(id)}
              </Badge>
            ))}
          </div>
        )}
      </div>

      {placement.rejected.length > 0 ? (
        <div className="space-y-1.5">
          <p className="text-sm font-medium text-foreground">
            {t('placement.rejectedTitle')}
          </p>
          <ul className="space-y-0.5 text-xs">
            {placement.rejected.map((r) => (
              <li key={r.node_id}>
                <span className="break-all font-mono text-foreground">
                  {nodeName(r.node_id)}
                </span>
                <span className="text-muted-foreground">: {r.reason}</span>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </>
  )
}
