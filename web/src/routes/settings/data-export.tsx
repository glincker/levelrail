import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ExportIcon } from '@phosphor-icons/react/dist/ssr'
import { PageHeader } from '@/components/shell/PageHeader'
import { Alert, AlertDescription } from '@/components/ui/alert'
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
    <div className="space-y-6">
      <PageHeader
        title={t('title')}
        description={t('description')}
        actions={notConfigured ? null : <CreateDataExportDestinationDialog />}
      />

      <Alert>
        <AlertDescription>
          <span className="font-medium">{t('scrape.title')}</span>
          {': '}
          <code className="font-mono text-xs">{t('scrape.path')}</code>
          <br />
          {t('scrape.description')}
        </AlertDescription>
      </Alert>

      {isPending ? (
        <div className="space-y-4" aria-hidden="true">
          <Skeleton className="h-40 w-full" />
          <Skeleton className="h-40 w-full" />
        </div>
      ) : null}

      {notConfigured ? (
        <Alert variant="destructive">
          <AlertDescription>{t('notConfigured')}</AlertDescription>
        </Alert>
      ) : null}

      {error && !notConfigured ? (
        <Alert variant="destructive">
          <AlertDescription>
            {t('loadError', { message: error.message })}
          </AlertDescription>
        </Alert>
      ) : null}

      {data && data.length === 0 ? (
        <EmptyState
          icon={<ExportIcon className="size-5" />}
          title={t('empty.title')}
          description={t('empty.description')}
          action={<CreateDataExportDestinationDialog />}
        />
      ) : null}

      {data && data.length > 0 ? (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          {data.map((d) => (
            <DataExportDestinationCard key={d.id} destination={d} />
          ))}
        </div>
      ) : null}
    </div>
  )
}
