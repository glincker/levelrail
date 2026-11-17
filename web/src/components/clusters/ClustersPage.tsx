import { useTranslation } from 'react-i18next'
import { BoxesGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { EmptyState } from '@/components/ui/empty-state'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { isClustersOff, useClusters } from '../../queries/clusters'
import { ClusterRow } from './ClusterRow'
import { ConnectClusterForm } from './ConnectClusterForm'

export function ClustersPage() {
  const { t } = useTranslation('clusters')
  const { data, isPending, isError, error } = useClusters()
  const off = isError && isClustersOff(error)

  return (
    <PageTemplate
      icon={<BoxesGlassIcon size="1.4em" />}
      title={t('title')}
      description={t('description')}
      width="wide"
    >
      {off ? (
        <EmptyState
          className="gap-2 border-0 bg-transparent py-4"
          icon={<BoxesGlassIcon className="size-6" />}
          title={t('disabled.title')}
          description={t('disabled.description')}
        />
      ) : isPending ? (
        <TableSkeleton columnCount={4} />
      ) : isError ? (
        <p className="text-sm text-destructive">{t('loadFailed')}</p>
      ) : (
        <div className="flex flex-col gap-4">
          {data.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              {t('empty.description')}
            </p>
          ) : (
            <ul
              aria-label={t('title')}
              className="divide-y divide-border rounded-lg border border-border bg-card"
            >
              {data.map((c) => (
                <ClusterRow key={c.name} cluster={c} />
              ))}
            </ul>
          )}
          <ConnectClusterForm existing={data.map((c) => c.name)} />
        </div>
      )}
    </PageTemplate>
  )
}
