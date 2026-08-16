import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { UsersIcon } from '@phosphor-icons/react/dist/ssr'
import { userListQueryOptions } from '../../queries/users'
import { UserTable } from '../../components/UserTable'

// AbilityRead-gated (GET /api/v1/users): who has access to this
// control plane. Every user has identical access, there is no
// invite flow yet (see internal/api/users.go's own doc comment).
export const Route = createFileRoute('/settings/users')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(userListQueryOptions()),
  component: UsersSettingsPage,
})

function UsersSettingsPage() {
  const { data: users } = useSuspenseQuery(userListQueryOptions())

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <UsersIcon className="size-4" />
        </div>
        <div>
          <h1 className="text-lg font-semibold text-foreground">Users</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Everyone with access to this platform. Every signed-in account
            has identical access.
          </p>
        </div>
      </div>
      <UserTable users={users} />
    </div>
  )
}
