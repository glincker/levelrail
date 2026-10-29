import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ShareGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Banner } from '@/components/ui/banner'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { ApiError } from '../../lib/apiError'
import { useDataExportDestinations } from '../../queries/dataExport'
import { DataExportDestinationCard } from '../../components/dataExport/DataExportDestinationCard'
import { CreateDataExportDestinationDialog } from '../../components/dataExport/CreateDataExportDestinationDialog'

export const Route = createFileRoute('/settings/data-export')({
  component: DataExportPage,
})

function DataExportPage() {
  const { t } = useTranslation('dataExport')
  const { data, isPending, error } = useDataExportDestinations()
  const notConfigured = error instanceof ApiError && error.status === 501

  return (
    <PageTemplate
      icon={<ShareGlassIcon size="1.4em" />}
      title={t('title')}
      description={t('description')}
      actions={notConfigured ? null : <CreateDataExportDestinationDialog />}
      banners={
        <Banner
          variant="info"
          title={`${t('scrape.title')}: ${t('scrape.path')}`}
        >
          {t('scrape.description')}
        </Banner>
      }
    >
      {isPending ? (
        <div className="space-y-4" aria-hidden="true">
          <Skeleton className="h-40 w-full" />
          <Skeleton className="h-40 w-full" />
        </div>
      ) : null}

      {notConfigured ? (
        <Banner variant="critical" title={t('notConfigured')} />
      ) : null}

      {error && !notConfigured ? (
        <Banner
          variant="critical"
          title={t('loadError', { message: error.message })}
        />
      ) : null}

      {data && data.length === 0 ? (
        <EmptyState
          icon={<ShareGlassIcon className="size-6" />}
          title={t('empty.title')}
          description={t('empty.description')}
          className="border-0 bg-transparent py-6"
        />
      ) : null}

      {data && data.length > 0 ? (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          {data.map((d) => (
            <DataExportDestinationCard key={d.id} destination={d} />
          ))}
        </div>
      ) : null}
    </PageTemplate>
  )
}
