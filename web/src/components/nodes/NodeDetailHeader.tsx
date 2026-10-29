import { useState, type ReactNode } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArrowLeftGlassIcon,
  EllipsisGlassIcon,
  ServerGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { ActionMenu } from '@/components/kit'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { PageBanner } from '../PageBanner'
import { LazyNodesDiagram } from '../banners/LazyInfraDiagrams'
import type { NodeResource } from '../../types/nodeDetail'
import { CordonNodeDialog } from '../CordonNodeDialog'
import { DeleteNodeDialog } from '../DeleteNodeDialog'
import { DrainNodeDialog } from '../DrainNodeDialog'

// Same action bar shape as the app page: the common actions inline, Delete
// behind the overflow menu so it never carries the weight of Drain.
export function NodeDetailHeader({
  node,
  children,
}: {
  node: NodeResource
  children?: ReactNode
}) {
  const { t } = useTranslation('nodes')
  const navigate = useNavigate()
  const [deleteOpen, setDeleteOpen] = useState(false)

  const size = node.status === 'pending' ? 'hero' : 'compact'
  const state = node.schedulable ? node.status : 'cordoned'

  return (
    <div className="mx-auto flex w-full min-w-0 max-w-(--glinr-content-max) flex-col gap-(--glinr-page-gap)">
      <div>
        <Link
          to="/nodes"
          className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
        >
          <ArrowLeftGlassIcon className="size-3.5" aria-hidden="true" />
          {t('detail.back')}
        </Link>
      </div>
      <PageBanner
        icon={<ServerGlassIcon size="1.2em" />}
        titleKey={node.name}
        descriptionKey={`nodes:detail.banner.${state}`}
        illustration={<LazyNodesDiagram size={size} />}
        size={size}
        action={
          <>
            {node.is_local ? (
              <Badge variant="info">{t('row.thisServer')}</Badge>
            ) : null}
            <CordonNodeDialog node={node} />
            <DrainNodeDialog node={node} />
            <ActionMenu
              trigger={
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t('row.actions', { name: node.name })}
                >
                  <EllipsisGlassIcon size="1.2em" aria-hidden="true" />
                </Button>
              }
              items={[
                {
                  id: 'delete',
                  label: t('delete.trigger'),
                  icon: <TrashGlassIcon size="1.2em" />,
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
      {children}
    </div>
  )
}
