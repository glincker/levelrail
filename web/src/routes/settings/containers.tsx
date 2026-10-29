import { createFileRoute } from '@tanstack/react-router'
import {
  BoxGlassIcon,
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  CircleMinusGlassIcon,
  LayersGlassIcon,
} from '@thesvg/ui-react'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { Badge } from '@/components/ui/badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { EmptyState } from '@/components/ui/empty-state'
import { ListSkeleton } from '@/components/ui/list-skeleton'
import { ApiError } from '../../lib/apiError'
import { useContainers } from '../../queries/containers'
import type { ContainerPort, ContainerResource } from '../../queries/containers'
import { ContainerRowActions } from '../../components/ContainerRowActions'
import { useTranslation } from 'react-i18next'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { OrphanReaperCard } from '../../components/OrphanReaperCard'

// Web equivalent of "levelrail-cli containers": GET
// /api/v1/system/containers, every container Docker knows about on this
// node whether or not it's managed by this platform. A Managed container
// stays read-only here (see internal/api/containers.go's own doc
// comment): stop/start/restart it from its own app page instead, which
// updates desired state correctly. An Orphaned one has no desired state
// to fight, so ContainerRowActions offers stop/remove/claim for it.
export const Route = createFileRoute('/settings/containers')({
  component: ContainersPage,
})

function formatPorts(ports: ContainerPort[]): string {
  if (ports.length === 0) return '-'
  return ports
    .map((p) => `${p.host_port}->${p.container_port}/${p.protocol}`)
    .join(', ')
}

const COLUMNS: ResponsiveColumn<ContainerResource>[] = [
  {
    id: 'name',
    header: 'Name',
    primary: true,
    cell: (c) => (
      <span
        className="block truncate font-medium text-foreground"
        title={c.name}
      >
        {c.name}
      </span>
    ),
  },
  {
    id: 'status',
    header: 'Status',
    width: 'w-28',
    cell: (c) => (
      <Badge variant={c.running ? 'success' : 'muted'}>
        {c.running ? (
          <CircleCheckGlassIcon className="text-tone-success" size="1.4em" />
        ) : (
          <CircleMinusGlassIcon size="1.4em" />
        )}
        {c.running ? 'Running' : 'Stopped'}
      </Badge>
    ),
  },
  {
    id: 'ownership',
    header: 'Ownership',
    width: 'w-28',
    priority: 2,
    cell: (c) => (
      <Badge variant={c.managed ? 'outline' : 'warning'}>
        {c.managed ? 'Managed' : 'Orphaned'}
      </Badge>
    ),
  },
  {
    id: 'image',
    header: 'Image',
    priority: 2,
    cell: (c) => (
      <span
        className="block truncate font-mono text-muted-foreground"
        title={c.image}
      >
        {c.image}
      </span>
    ),
  },
  {
    id: 'ports',
    header: 'Ports',
    priority: 3,
    cell: (c) => (
      <span
        className="block truncate font-mono text-muted-foreground"
        title={formatPorts(c.ports)}
      >
        {formatPorts(c.ports)}
      </span>
    ),
  },
]

function ContainersPage() {
  const { t } = useTranslation('settings')
  const { data: containers, isLoading, isError, error } = useContainers()

  return (
    <PageTemplate
      icon={<LayersGlassIcon size="1.4em" />}
      title={t('containersPage.title')}
      description={t('containersPage.description')}
    >
      <OrphanReaperCard />

      {isLoading ? <ListSkeleton rows={5} /> : null}

      {isError ? (
        error instanceof ApiError && error.status === 501 ? (
          <Alert>
            <CircleAlertGlassIcon className="text-tone-danger" size="1.4em" />
            <AlertTitle>{t('containersPage.notAvailable')}</AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        ) : (
          <Alert variant="destructive">
            <CircleAlertGlassIcon className="text-tone-danger" size="1.4em" />
            <AlertTitle>{t('containersPage.listFailed')}</AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        )
      ) : null}

      {!isLoading && !isError && containers ? (
        containers.length === 0 ? (
          <EmptyState
            className="gap-2 border-0 bg-transparent py-4"
            icon={<BoxGlassIcon className="size-6" />}
            title={t('containersPage.emptyTitle')}
            description={t('containersPage.emptyDescription')}
          />
        ) : (
          <ResponsiveTable
            rows={containers}
            columns={COLUMNS}
            getRowId={(c) => c.name}
            ariaLabel={t('containersPage.title')}
            renderActions={(c) =>
              c.managed ? null : <ContainerRowActions name={c.name} />
            }
          />
        )
      ) : null}
    </PageTemplate>
  )
}
