import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { DatabaseIcon, PlusIcon } from '@phosphor-icons/react/dist/ssr'
import { useEnvironmentScope } from '../../lib/environmentScope'
import { routeErrorMessage } from '../../lib/apiError'
import { databaseListQueryOptions } from '../../queries/databases'
import { DATABASE_LIST_GRID, RowSkeleton } from '../../components/DatabaseRow'
import { DatabasesTable } from '../../components/DatabasesTable'
import { CreateResourceWizard } from '../../components/CreateResourceWizard'
import { ConnectDatabaseDialog } from '../../components/ConnectDatabaseDialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '../../components/ui/button'
import { EmptyState } from '../../components/ui/empty-state'
import { PageTemplate } from '../../components/shell/PageTemplate'

// Loader primes the cache; the component reads it via useSuspenseQuery.
// Virtualized unconditionally: GET /api/v1/databases returns everything.
export const Route = createFileRoute('/databases/')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(databaseListQueryOptions()),
  component: DatabaseListPage,
  pendingComponent: DatabaseListPending,
  errorComponent: DatabaseListError,
})

function ListHeader() {
  const { t } = useTranslation('databases')
  return (
    <div
      className={`${DATABASE_LIST_GRID} sticky top-0 z-10 border-b border-border bg-card px-4 py-2 text-xs font-medium tracking-wide text-muted-foreground uppercase`}
    >
      <span aria-hidden="true" />
      <span>{t('list.colName')}</span>
      <span className="hidden lg:block">{t('list.colEngine')}</span>
      <span className="hidden lg:block">{t('list.colBackups')}</span>
      <span className="hidden lg:block">{t('list.colNode')}</span>
      <span aria-hidden="true" />
    </div>
  )
}

function NewDatabaseButton() {
  const { t } = useTranslation('databases')
  return (
    <CreateResourceWizard
      scope="databases"
      trigger={
        <Button size="sm">
          <PlusIcon aria-hidden="true" />
          {t('list.new')}
        </Button>
      }
    />
  )
}

function DatabaseListPage() {
  const { t } = useTranslation('databases')
  useEnvironmentScope()
  const { data: databases } = useSuspenseQuery(databaseListQueryOptions())
  return (
    <PageTemplate
      title={t('list.title')}
      description={
        databases.length > 0
          ? t('list.count', { count: databases.length })
          : undefined
      }
      actions={
        <>
          <ConnectDatabaseDialog />
          <NewDatabaseButton />
        </>
      }
    >
      {databases.length === 0 ? (
        <EmptyState
          icon={<DatabaseIcon className="size-5" />}
          title={t('list.emptyTitle')}
          description={t('list.emptyBody')}
          action={<NewDatabaseButton />}
        />
      ) : (
        <DatabasesTable databases={databases} />
      )}
    </PageTemplate>
  )
}

function DatabaseListPending() {
  const { t } = useTranslation('databases')
  return (
    <PageTemplate title={t('list.title')}>
      <div className="overflow-hidden rounded-lg border border-border bg-card">
        <ListHeader />
        {Array.from({ length: 6 }, (_, i) => (
          <RowSkeleton key={i} />
        ))}
      </div>
    </PageTemplate>
  )
}

function DatabaseListError({ error }: { error: unknown }) {
  const { t } = useTranslation('databases')
  return (
    <PageTemplate title={t('list.title')}>
      <Alert variant="destructive">
        <AlertDescription className="space-y-2">
          <p className="font-medium">{t('list.errorTitle')}</p>
          <p>
            {t('list.errorBody')} {routeErrorMessage(error)}
          </p>
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              globalThis.location.reload()
            }}
          >
            {t('list.retry')}
          </Button>
        </AlertDescription>
      </Alert>
    </PageTemplate>
  )
}
