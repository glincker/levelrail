import { createFileRoute } from '@tanstack/react-router'
import { CircleAlertGlassIcon, UploadGlassIcon } from '@thesvg/ui-react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  storageDestinationsQueryOptions,
  storageProvidersQueryOptions,
} from '../../queries/storage'
import { ApiError } from '../../lib/apiError'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { PageHeader } from '@/components/shell/PageHeader'
import { CreateStorageDestinationDialog } from '../../components/CreateStorageDestinationDialog'
import { CreateNodeStorageDestinationDialog } from '../../components/CreateNodeStorageDestinationDialog'
import { StorageDefaultsCard } from '../../components/StorageDefaultsCard'
import { StorageDestinationTable } from '../../components/StorageDestinationTable'
import { LogArchivePanel } from '../../components/LogArchivePanel'
import { BuildCacheCard } from '../../components/BuildCacheCard'

export const Route = createFileRoute('/settings/storage')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(storageProvidersQueryOptions()),
  component: StoragePage,
})

function StoragePage() {
  const { t } = useTranslation('settings')
  const destinations = useQuery({
    ...storageDestinationsQueryOptions(),
    retry: false,
  })
  const notConfigured =
    destinations.error instanceof ApiError && destinations.error.status === 501

  return (
    <div className="space-y-8">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <UploadGlassIcon className="size-5" aria-hidden="true" />
        </div>
        <div className="min-w-0 flex-1">
          <PageHeader
            title={t('storagePage.title')}
            description={t('storagePage.description')}
            actions={
              notConfigured ? null : (
                <div className="flex flex-wrap gap-2">
                  <CreateNodeStorageDestinationDialog />
                  <CreateStorageDestinationDialog />
                </div>
              )
            }
          />
        </div>
      </div>

      {notConfigured ? (
        <Alert>
          <AlertTitle>{t('storagePage.notConfiguredTitle')}</AlertTitle>
          <AlertDescription>
            {t('storagePage.notConfiguredBody')}
          </AlertDescription>
        </Alert>
      ) : destinations.isError ? (
        <EmptyState
          icon={<CircleAlertGlassIcon className="size-6 text-tone-danger" />}
          title={t('storagePage.loadErrorTitle')}
          description={destinations.error.message}
          action={
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                void destinations.refetch()
              }}
            >
              {t('storagePage.retry')}
            </Button>
          }
        />
      ) : destinations.data ? (
        <>
          <StorageDestinationTable
            destinations={destinations.data}
            action={<CreateStorageDestinationDialog />}
          />
          <StorageDefaultsCard destinations={destinations.data} />
          <div className="space-y-3">
            <h2 className="text-base font-semibold text-foreground">
              {t('storagePage.logArchiveTitle')}
            </h2>
            <p className="text-sm text-muted-foreground">
              {t('storagePage.logArchiveBody')}
            </p>
            <LogArchivePanel appName="" />
          </div>
          <div className="space-y-3">
            <h2 className="text-base font-semibold text-foreground">
              {t('storagePage.buildCacheTitle')}
            </h2>
            <BuildCacheCard appName="" />
          </div>
        </>
      ) : null}
    </div>
  )
}
