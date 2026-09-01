import { createFileRoute } from '@tanstack/react-router'
import { TerminalWindowIcon } from '@phosphor-icons/react/dist/ssr'
import {
  deviceAuthRequestsQueryOptions,
  useDeviceAuthRequests,
} from '../../queries/deviceAuth'
import { DeviceAuthRequestTable } from '../../components/DeviceAuthRequestTable'
import { TableSkeleton } from '@/components/ui/table-skeleton'

interface CliAccessSearch {
  user_code?: string
}

// A plain function, not zod, matching reset-password.tsx's own reasoning:
// validateSearch runs as part of eager route matching, so pulling zod in
// here for one optional string would undo autoCodeSplitting's work.
function validateCliAccessSearch(
  search: Record<string, unknown>,
): CliAccessSearch {
  const { user_code } = search
  return typeof user_code === 'string' ? { user_code } : {}
}

// Web half of "levelrail-cli auth login --device": the CLI prints a
// user_code and polls POST /api/v1/auth/device/token, this page is
// where an operator sees that same code and approves or denies it.
// device_auth.go's VerificationURIComplete already points the CLI's
// printed link at /settings/cli-access?user_code=..., which
// validateSearch reads to highlight the matching row.
export const Route = createFileRoute('/settings/cli-access')({
  validateSearch: validateCliAccessSearch,
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(deviceAuthRequestsQueryOptions()),
  component: CliAccessPage,
  pendingComponent: CliAccessPending,
})

function CliAccessPage() {
  const { data: requests } = useDeviceAuthRequests()
  const { user_code: highlightUserCode } = Route.useSearch()

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <TerminalWindowIcon className="size-4" />
        </div>
        <div>
          <h1 className="text-lg font-semibold text-foreground">
            CLI access
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Approve or deny a login started with levelrail-cli auth login
            --device. Run that command in a terminal, then match the code it
            prints against the code shown below before approving it.
          </p>
        </div>
      </div>
      <DeviceAuthRequestTable
        requests={requests}
        highlightUserCode={highlightUserCode}
      />
    </div>
  )
}

function CliAccessPending() {
  return (
    <div className="space-y-6">
      <h1 className="text-lg font-semibold text-foreground">CLI access</h1>
      <TableSkeleton columnCount={5} />
    </div>
  )
}
