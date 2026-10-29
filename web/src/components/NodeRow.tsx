import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  EllipsisGlassIcon,
  LockGlassIcon,
  LockOpenGlassIcon,
  MaximizeGlassIcon,
  ServerGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { ActionMenu } from '@/components/kit'
import { StatusPill } from '@/components/kit/StatusPill'
import type { Tone } from '@/components/kit/tone'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { formatAge } from '../lib/format'
import type { NodeResource, NodeStatus } from '../types/nodeDetail'
import type { NodeResourceUsage } from '../types/fleetResourceUsage'
import { CordonNodeDialog } from './CordonNodeDialog'
import { DeleteNodeDialog } from './DeleteNodeDialog'
import { DrainNodeDialog } from './DrainNodeDialog'
import { NodeCertBadge } from './NodeCertBadge'
import { NodeNeverConnectedBadge } from './NodeNeverConnected'
import { NodeCPUCell, NodeMemoryCell } from './NodeUtilizationCells'

// Same anatomy as DATABASE_LIST_GRID: icon, name with status beneath, lg-only
// columns, trailing kebab. Shared by the sticky header and every row.
export const NODE_LIST_GRID =
  'grid grid-cols-[2rem_minmax(0,1fr)_2rem] lg:grid-cols-[2rem_minmax(0,1.5fr)_minmax(0,0.9fr)_minmax(0,1.1fr)_4.5rem_minmax(0,0.9fr)_minmax(0,0.8fr)_2rem] items-center gap-3'

// Connectivity and cordon are independent axes: nothing writes status
// "cordoned", schedulable carries that state, so the row shows both.
const STATUS_TONE: Record<NodeStatus, Tone> = {
  pending: 'neutral',
  online: 'success',
  offline: 'danger',
  cordoned: 'warning',
}

export function NodeRow({
  node,
  usage,
  resourceCount,
}: {
  node: NodeResource
  usage?: NodeResourceUsage
  resourceCount?: number
}) {
  const { t } = useTranslation('nodes')
  const [dialog, setDialog] = useState<'cordon' | 'drain' | 'delete' | null>(
    null,
  )
  const close = (open: boolean) => {
    if (!open) {
      setDialog(null)
    }
  }
  const lastSeen = node.last_seen_at
    ? new Date(node.last_seen_at).toLocaleString()
    : t('list.never')

  return (
    <div
      className={`${NODE_LIST_GRID} group/row relative h-full w-full border-b border-border px-4 py-3 transition-colors hover:bg-muted/60`}
    >
      <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
        <ServerGlassIcon className="size-5" aria-hidden="true" />
      </span>

      <span className="flex min-w-0 flex-col justify-center gap-1">
        <Link
          to="/nodes/$id"
          params={{ id: node.id }}
          className="truncate text-sm font-medium text-foreground after:absolute after:inset-0"
        >
          {node.name}
        </Link>
        <span className="flex min-w-0 flex-wrap items-center gap-1.5">
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
            <Badge variant={node.gpu.runtime_installed ? 'outline' : 'warning'}>
              {t('row.gpuFree', {
                free: node.gpu.free_gpus,
                total: node.gpu.gpu_count,
              })}
            </Badge>
          ) : null}
          <NodeNeverConnectedBadge node={node} />
          <NodeCertBadge cert={node.cert} showOk={false} />
          <span className="min-w-0 truncate font-mono text-xs text-muted-foreground lg:hidden">
            {node.address ?? ''}
          </span>
        </span>
      </span>

      <span className="hidden min-w-0 truncate font-mono text-xs text-muted-foreground lg:block">
        {node.address ?? t('list.addressNotSet')}
      </span>

      <span className="hidden min-w-0 lg:grid lg:gap-2">
        <NodeCPUCell usage={usage} />
        <NodeMemoryCell usage={usage} />
      </span>

      <span className="hidden text-xs text-muted-foreground tabular-nums lg:block">
        {resourceCount === undefined
          ? '-'
          : t('list.resourceCount', { count: resourceCount })}
      </span>

      <span className="hidden min-w-0 flex-wrap items-center gap-1 text-xs text-muted-foreground lg:flex">
        <span className="font-mono">
          {node.agent?.version ?? t('list.agentUnknown')}
        </span>
        {node.agent?.outdated ? (
          <Badge
            variant="warning"
            title={t('row.outdatedHint', { min: node.agent.min_version ?? '' })}
          >
            {t('row.outdated')}
          </Badge>
        ) : null}
      </span>

      <span
        className="hidden min-w-0 truncate text-xs text-muted-foreground lg:block"
        title={lastSeen}
      >
        {formatAge(node.last_seen_at, t('list.never'))}
      </span>

      <span className="relative z-10 justify-self-end">
        <ActionMenu
          trigger={
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t('row.actions', { name: node.name })}
            >
              <EllipsisGlassIcon size="1.4em" aria-hidden="true" />
            </Button>
          }
          items={[
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
                setDialog('cordon')
              },
            },
            {
              id: 'drain',
              label: t('drain.trigger'),
              icon: <MaximizeGlassIcon size="1.4em" />,
              onSelect: () => {
                setDialog('drain')
              },
            },
            {
              id: 'delete',
              label: t('delete.trigger'),
              icon: <TrashGlassIcon size="1.4em" />,
              tone: 'danger',
              onSelect: () => {
                setDialog('delete')
              },
            },
          ]}
        />
      </span>

      <CordonNodeDialog
        node={node}
        control={{
          open: dialog === 'cordon',
          onOpenChange: close,
          hideTrigger: true,
        }}
      />
      <DrainNodeDialog
        node={node}
        control={{
          open: dialog === 'drain',
          onOpenChange: close,
          hideTrigger: true,
        }}
      />
      <DeleteNodeDialog
        id={node.id}
        name={node.name}
        control={{
          open: dialog === 'delete',
          onOpenChange: close,
          hideTrigger: true,
        }}
      />
    </div>
  )
}

// Matches NODE_LIST_GRID so the skeleton does not jump when rows swap in.
export function RowSkeleton() {
  return (
    <div
      className={`${NODE_LIST_GRID} border-b border-border px-4 py-3`}
      aria-hidden="true"
    >
      <div className="size-8 animate-pulse rounded-lg bg-muted" />
      <div className="h-4 w-32 animate-pulse rounded bg-muted" />
      <div className="hidden h-4 w-24 animate-pulse rounded bg-muted lg:block" />
      <div className="hidden h-4 w-24 animate-pulse rounded bg-muted lg:block" />
      <div className="hidden h-4 w-10 animate-pulse rounded bg-muted lg:block" />
      <div className="hidden h-4 w-16 animate-pulse rounded bg-muted lg:block" />
      <div className="hidden h-4 w-16 animate-pulse rounded bg-muted lg:block" />
      <div className="size-7 animate-pulse justify-self-end rounded bg-muted" />
    </div>
  )
}
