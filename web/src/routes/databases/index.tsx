import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { DatabaseIcon, PlusIcon } from '@phosphor-icons/react/dist/ssr'
import { useEnvironmentScope } from '../../lib/environmentScope'
import { routeErrorMessage } from '../../lib/apiError'
import { databaseListQueryOptions } from '../../queries/databases'
import {
  DATABASE_LIST_GRID,
  DatabaseRow,
  RowSkeleton,
} from '../../components/DatabaseRow'
import { CreateResourceWizard } from '../../components/CreateResourceWizard'
import { ConnectDatabaseDialog } from '../../components/ConnectDatabaseDialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '../../components/ui/button'
import { EmptyState } from '../../components/ui/empty-state'
import { PageHeader } from '../../components/shell/PageHeader'

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
  const parentRef = useRef<HTMLDivElement>(null)

  const virtualizer = useVirtualizer({
    count: databases.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 60,
    overscan: 8,
  })

  return (
    <div className="space-y-4">
      <PageHeader
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
      />
      {databases.length === 0 ? (
        <EmptyState
          icon={<DatabaseIcon className="size-5" />}
          title={t('list.emptyTitle')}
          description={t('list.emptyBody')}
          action={<NewDatabaseButton />}
        />
      ) : (
        <div
          ref={parentRef}
          className="max-h-[70vh] overflow-auto rounded-lg border border-border bg-card"
        >
          <ListHeader />
          <div
            style={{
              height: virtualizer.getTotalSize(),
              position: 'relative',
            }}
          >
            {virtualizer.getVirtualItems().map((virtualRow) => {
              const database = databases[virtualRow.index]
              if (!database) {
                return null
              }
              return (
                <div
                  key={virtualRow.key}
                  data-index={virtualRow.index}
                  ref={virtualizer.measureElement}
                  style={{
                    position: 'absolute',
                    top: 0,
                    left: 0,
                    width: '100%',
                    transform: `translateY(${virtualRow.start}px)`,
                  }}
                >
                  <DatabaseRow database={database} />
                </div>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}

function DatabaseListPending() {
  const { t } = useTranslation('databases')
  return (
    <div className="space-y-4">
      <PageHeader title={t('list.title')} />
      <div className="overflow-hidden rounded-lg border border-border bg-card">
        <ListHeader />
        {Array.from({ length: 6 }, (_, i) => (
          <RowSkeleton key={i} />
        ))}
      </div>
    </div>
  )
}

function DatabaseListError({ error }: { error: unknown }) {
  const { t } = useTranslation('databases')
  return (
    <div className="space-y-4">
      <PageHeader title={t('list.title')} />
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
    </div>
  )
}
