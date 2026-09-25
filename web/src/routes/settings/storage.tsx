import { createFileRoute } from '@tanstack/react-router'
import { CloudArrowUpIcon } from '@phosphor-icons/react/dist/ssr'
import { useQuery } from '@tanstack/react-query'
import {
  storageDestinationsQueryOptions,
  storageProvidersQueryOptions,
} from '../../queries/storage'
import { ApiError } from '../../lib/apiError'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { CreateStorageDestinationDialog } from '../../components/CreateStorageDestinationDialog'
import { StorageDestinationTable } from '../../components/StorageDestinationTable'
import { LogArchivePanel } from '../../components/LogArchivePanel'

export const Route = createFileRoute('/settings/storage')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(storageProvidersQueryOptions()),
  component: StoragePage,
})

function StoragePage() {
  const destinations = useQuery({
    ...storageDestinationsQueryOptions(),
    retry: false,
  })
  const notConfigured =
    destinations.error instanceof ApiError && destinations.error.status === 501

  return (
    <div className="space-y-8">
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <CloudArrowUpIcon className="size-4" aria-hidden="true" />
          </div>
          <div>
            <h1 className="text-lg font-semibold text-foreground">
              Storage destinations
            </h1>
            <p className="mt-1 text-sm text-muted-foreground">
              S3-compatible object storage for log archives and backups. AWS S3,
              Cloudflare R2, Backblaze B2, MinIO, Wasabi, or any custom
              endpoint.
            </p>
          </div>
        </div>
        {notConfigured ? null : <CreateStorageDestinationDialog />}
      </div>

      {notConfigured ? (
        <Alert>
          <AlertTitle>Storage is not configured on this server</AlertTitle>
          <AlertDescription>
            Set APP_MASTER_KEY and restart the control plane so bucket
            credentials can be encrypted.
          </AlertDescription>
        </Alert>
      ) : destinations.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {destinations.error.message}
        </p>
      ) : destinations.data ? (
        <>
          <StorageDestinationTable
            destinations={destinations.data}
            action={<CreateStorageDestinationDialog />}
          />
          <div className="space-y-3">
            <h2 className="text-base font-semibold text-foreground">
              Log archive for all apps
            </h2>
            <p className="text-sm text-muted-foreground">
              Ships node-local logs to a destination on a schedule. Apps with
              their own policy (app Logs, Archive tab) are kept separate.
            </p>
            <LogArchivePanel appName="" />
          </div>
        </>
      ) : null}
    </div>
  )
}
