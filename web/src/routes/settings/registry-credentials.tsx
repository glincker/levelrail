import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { PackageIcon } from '@phosphor-icons/react/dist/ssr'
import { registryCredentialListQueryOptions } from '../../queries/registryCredentials'
import { RegistryCredentialTable } from '../../components/RegistryCredentialTable'
import { CreateRegistryCredentialDialog } from '../../components/CreateRegistryCredentialDialog'
import { TableSkeleton } from '@/components/ui/table-skeleton'

// Account-level, same reasoning settings/backup-targets.tsx's own doc
// comment gives: a registry credential isn't scoped to one app, it's
// referenced by name from any app.yaml's build.registryCredential
// field, so it lives here rather than under routes/apps/.
export const Route = createFileRoute('/settings/registry-credentials')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(registryCredentialListQueryOptions()),
  component: RegistryCredentialsPage,
  pendingComponent: RegistryCredentialsPending,
})

function RegistryCredentialsPage() {
  const { data: credentials } = useSuspenseQuery(
    registryCredentialListQueryOptions(),
  )

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <PackageIcon className="size-4" />
          </div>
          <div>
            <h1 className="text-lg font-semibold text-foreground">
              Registry credentials
            </h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Usernames and passwords for pulling private images with
              build.type: image, referenced by name from app.yaml.
            </p>
          </div>
        </div>
        <CreateRegistryCredentialDialog />
      </div>
      <RegistryCredentialTable credentials={credentials} />
    </div>
  )
}

// Route-level fallback for the loader's pending phase, matching
// RegistryCredentialTable's own 5-column shape so the skeleton doesn't
// jump when real rows swap in.
function RegistryCredentialsPending() {
  return (
    <div className="space-y-6">
      <h1 className="text-lg font-semibold text-foreground">
        Registry credentials
      </h1>
      <TableSkeleton columnCount={5} />
    </div>
  )
}
