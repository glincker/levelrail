import { useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArrowLeftIcon,
  DotsThreeIcon,
  TrashIcon,
} from '@phosphor-icons/react/dist/ssr'
import { ActionMenu } from '@/components/kit'
import { StatusPill } from '@/components/kit/StatusPill'
import type { Tone } from '@/components/kit/tone'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { PageHeader } from '@/components/shell/PageHeader'
import type { NodeResource, NodeStatus } from '../../types/nodeDetail'
import { CordonNodeDialog } from '../CordonNodeDialog'
import { DeleteNodeDialog } from '../DeleteNodeDialog'
import { DrainNodeDialog } from '../DrainNodeDialog'

const STATUS_TONE: Record<NodeStatus, Tone> = {
  pending: 'neutral',
  online: 'success',
  offline: 'danger',
  cordoned: 'warning',
}

// Same action bar shape as the app page: the common actions inline, Delete
// behind the overflow menu so it never carries the weight of Drain.
export function NodeDetailHeader({ node }: { node: NodeResource }) {
  const { t } = useTranslation('nodes')
  const navigate = useNavigate()
  const [deleteOpen, setDeleteOpen] = useState(false)

  return (
    <PageHeader
      breadcrumb={
        <Link
          to="/nodes"
          className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
        >
          <ArrowLeftIcon className="size-3" aria-hidden="true" />
          {t('list.title')}
        </Link>
      }
      title={node.name}
      status={
        <>
          <StatusPill
            tone={STATUS_TONE[node.status]}
            label={t(`status.${node.status}`)}
          />
          {node.is_local ? (
            <Badge variant="info">{t('row.thisServer')}</Badge>
          ) : null}
          {node.schedulable ? null : (
            <Badge variant="warning">{t('status.cordoned')}</Badge>
          )}
        </>
      }
      actions={
        <>
          <CordonNodeDialog node={node} />
          <DrainNodeDialog node={node} />
          <ActionMenu
            trigger={
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t('row.actions', { name: node.name })}
              >
                <DotsThreeIcon aria-hidden="true" />
              </Button>
            }
            items={[
              {
                id: 'delete',
                label: t('delete.trigger'),
                icon: <TrashIcon />,
                tone: 'danger',
                onSelect: () => {
                  setDeleteOpen(true)
                },
              },
            ]}
          />
          <DeleteNodeDialog
            id={node.id}
            name={node.name}
            onDeleted={() => {
              void navigate({ to: '/nodes' })
            }}
            control={{
              open: deleteOpen,
              onOpenChange: setDeleteOpen,
              hideTrigger: true,
            }}
          />
        </>
      }
    />
  )
}
