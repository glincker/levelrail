import { useMemo, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  LockGlassIcon,
  LockOpenGlassIcon,
  MaximizeGlassIcon,
  ServerGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { StatusPill } from '@/components/kit/StatusPill'
import type { Tone } from '@/components/kit/tone'
import { Badge } from '@/components/ui/badge'
import {
  ROW_LINK_OVERLAY,
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { formatAge } from '../lib/format'
import type { NodeResource, NodeStatus } from '../types/nodeDetail'
import type { NodeResourceUsage } from '../types/fleetResourceUsage'
import { CordonNodeDialog } from './CordonNodeDialog'
import { DeleteNodeDialog } from './DeleteNodeDialog'
import { DrainNodeDialog } from './DrainNodeDialog'
import { NodeCertBadge } from './NodeCertBadge'
import { NodeNeverConnectedBadge } from './NodeNeverConnected'
import { NodeCPUCell, NodeMemoryCell } from './NodeUtilizationCells'

// Connectivity and cordon are independent axes: nothing writes status
// "cordoned", schedulable carries that state, so the row shows both.
const STATUS_TONE: Record<NodeStatus, Tone> = {
  pending: 'neutral',
  online: 'success',
  offline: 'danger',
  cordoned: 'warning',
}

type DialogKind = 'cordon' | 'drain' | 'delete'

export function NodesTable({
  nodes,
  usageByNodeId,
  counts,
}: {
  nodes: readonly NodeResource[]
  usageByNodeId: ReadonlyMap<string, NodeResourceUsage>
  counts: ReadonlyMap<string, number>
}) {
  const { t } = useTranslation('nodes')
  const [dialog, setDialog] = useState<{
    kind: DialogKind
    node: NodeResource
  } | null>(null)
  const close = (open: boolean) => {
    if (!open) setDialog(null)
  }

  const columns = useMemo<ResponsiveColumn<NodeResource>[]>(
    () => [
      {
        id: 'name',
        primary: true,
        header: t('list.name'),
        cell: (node) => (
          <span className="flex min-w-0 flex-col gap-1">
            <Link
              to="/nodes/$id"
              params={{ id: node.id }}
              className={`truncate text-sm font-medium text-foreground ${ROW_LINK_OVERLAY}`}
            >
              {node.name}
            </Link>
            <span className="flex flex-wrap items-center gap-1.5">
              <StatusPill
                size="sm"
                tone={STATUS_TONE[node.status]}
                label={t(`status.${node.status}`)}
              />
              {node.is_local ? (
                <Badge variant="info">{t('row.thisServer')}</Badge>
              ) : null}
              {node.schedulable ? null : (
                <Badge variant="warning">{t('status.cordoned')}</Badge>
              )}
              {node.accepts_app_workloads ? null : (
                <Badge variant="outline">{t('row.buildOnly')}</Badge>
              )}
              {node.gpu?.present ? (
                <Badge
                  variant={node.gpu.runtime_installed ? 'outline' : 'warning'}
                >
                  {t('row.gpuFree', {
                    free: node.gpu.free_gpus,
                    total: node.gpu.gpu_count,
                  })}
                </Badge>
              ) : null}
              {node.zone || node.region ? (
                <Badge variant="outline">
                  {[node.provider, node.region, node.zone]
                    .filter(Boolean)
                    .join(' / ')}
                </Badge>
              ) : null}
              <NodeNeverConnectedBadge node={node} />
              <NodeCertBadge cert={node.cert} showOk={false} />
            </span>
          </span>
        ),
      },
      {
        id: 'address',
        header: t('list.address'),
        priority: 2,
        width: 'w-44',
        cell: (node) => (
          <span className="block truncate font-mono text-xs text-muted-foreground">
            {node.address ?? t('list.addressNotSet')}
          </span>
        ),
      },
      {
        id: 'usage',
        header: t('list.usage'),
        priority: 2,
        width: 'w-44',
        cell: (node) => (
          <span className="grid min-w-0 gap-2">
            <NodeCPUCell usage={usageByNodeId.get(node.id)} />
            <NodeMemoryCell usage={usageByNodeId.get(node.id)} />
          </span>
        ),
      },
      {
        id: 'resources',
        header: t('list.resources'),
        priority: 3,
        width: 'w-28',
        cell: (node) => {
          const count = counts.get(node.id)
          return (
            <span className="text-xs text-muted-foreground tabular-nums">
              {count === undefined ? '-' : t('list.resourceCount', { count })}
            </span>
          )
        },
      },
      {
        id: 'agent',
        header: t('list.agent'),
        priority: 3,
        width: 'w-36',
        cell: (node) => (
          <span className="flex flex-wrap items-center gap-1 text-xs text-muted-foreground">
            <span className="font-mono">
              {node.agent?.version ?? t('list.agentUnknown')}
            </span>
            {node.agent?.outdated ? (
              <Badge
                variant="warning"
                title={t('row.outdatedHint', {
                  min: node.agent.min_version ?? '',
                })}
              >
                {t('row.outdated')}
              </Badge>
            ) : null}
          </span>
        ),
      },
      {
        id: 'lastSeen',
        header: t('list.lastSeen'),
        priority: 2,
        width: 'w-28',
        cell: (node) => (
          <span
            className="text-xs text-muted-foreground"
            title={
              node.last_seen_at
                ? new Date(node.last_seen_at).toLocaleString()
                : t('list.never')
            }
          >
            {formatAge(node.last_seen_at, t('list.never'))}
          </span>
        ),
      },
    ],
    [t, usageByNodeId, counts],
  )

  return (
    <>
      <ResponsiveTable
        rows={nodes}
        columns={columns}
        getRowId={(n) => n.id}
        ariaLabel={t('list.tableLabel')}
        leading={{
          header: <span className="sr-only">{t('list.name')}</span>,
          cell: () => (
            <span
              aria-hidden="true"
              className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground"
            >
              <ServerGlassIcon className="size-5" />
            </span>
          ),
        }}
        actionsLabel={(node) => t('row.actions', { name: node.name })}
        actions={(node) => [
          {
            id: 'cordon',
            label: node.schedulable
              ? t('cordon.trigger')
              : t('uncordon.trigger'),
            icon: node.schedulable ? (
              <LockGlassIcon size="1.4em" />
            ) : (
              <LockOpenGlassIcon size="1.4em" />
            ),
            onSelect: () => {
              setDialog({ kind: 'cordon', node })
            },
          },
          {
            id: 'drain',
            label: t('drain.trigger'),
            icon: <MaximizeGlassIcon size="1.4em" />,
            onSelect: () => {
              setDialog({ kind: 'drain', node })
            },
          },
          {
            id: 'delete',
            label: t('delete.trigger'),
            icon: <TrashGlassIcon size="1.4em" />,
            tone: 'danger',
            onSelect: () => {
              setDialog({ kind: 'delete', node })
            },
          },
        ]}
      />
      {dialog?.kind === 'cordon' ? (
        <CordonNodeDialog
          node={dialog.node}
          control={{ open: true, onOpenChange: close, hideTrigger: true }}
        />
      ) : null}
      {dialog?.kind === 'drain' ? (
        <DrainNodeDialog
          node={dialog.node}
          control={{ open: true, onOpenChange: close, hideTrigger: true }}
        />
      ) : null}
      {dialog?.kind === 'delete' ? (
        <DeleteNodeDialog
          id={dialog.node.id}
          name={dialog.node.name}
          control={{ open: true, onOpenChange: close, hideTrigger: true }}
        />
      ) : null}
    </>
  )
}
