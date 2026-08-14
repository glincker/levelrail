import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { CloudArrowUpIcon } from '@phosphor-icons/react/dist/ssr'
import { backupTargetListQueryOptions } from '../../queries/backupTargets'
import { BackupTargetTable } from '../../components/BackupTargetTable'
import { CreateBackupTargetDialog } from '../../components/CreateBackupTargetDialog'

// Account-level, not scoped to one app or database, so it lives under
// routes/settings/ next to tokens.tsx rather than under routes/apps/ or
// routes/databases/, same reasoning that route's own doc comment gives.
// Typed loader primes the Query cache before render, same pattern
// tokens.tsx already established: the component below only ever reads
// that warm cache via useSuspenseQuery, never fetches data in its own
// body.
//
// This page only covers connecting/listing/removing a bucket
// destination. Triggering an actual backup and viewing its history lives
// on the database that owns it (components/BackupsSection.tsx, rendered
// from routes/databases/$name/overview.tsx), not here: a backup target is
// account-level and can back up more than one database, so its own runs
// belong on the database's page, not this one.
export const Route = createFileRoute('/settings/backup-targets')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(backupTargetListQueryOptions()),
  component: BackupTargetsPage,
})

function BackupTargetsPage() {
  const { data: targets } = useSuspenseQuery(backupTargetListQueryOptions())

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <CloudArrowUpIcon className="size-4" />
          </div>
          <div>
            <h1 className="text-lg font-semibold text-foreground">
              Backup targets
            </h1>
            <p className="mt-1 text-sm text-muted-foreground">
              S3-compatible buckets connected as backup destinations for managed
              databases. AWS S3, Cloudflare R2, or any other S3-compatible
              endpoint.
            </p>
          </div>
        </div>
        <CreateBackupTargetDialog />
      </div>
      <BackupTargetTable targets={targets} />
    </div>
  )
}
