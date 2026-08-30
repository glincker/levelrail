import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { UsersIcon } from '@phosphor-icons/react/dist/ssr'
import { userListQueryOptions } from '../../queries/users'
import { roleListQueryOptions } from '../../queries/roles'
import { UserTable } from '../../components/UserTable'
import { CreateUserDialog } from '../../components/CreateUserDialog'
import { useAuthUsername } from '../../hooks/useAuthUsername'
import { TableSkeleton } from '@/components/ui/table-skeleton'

// AbilityRead-gated (GET /api/v1/users): who has access to this control
// plane. Abilities are now per-user (internal/api/users.go's own doc
// comment), shown and edited per row by UserTable, not implied by being
// signed in. Roles (GET /api/v1/roles) are preloaded here too, so
// CreateUserDialog/EditUserAbilitiesDialog's RoleSelect can read them via
// useSuspenseQuery without a second loading state of their own.
export const Route = createFileRoute('/settings/users')({
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.ensureQueryData(userListQueryOptions()),
      queryClient.ensureQueryData(roleListQueryOptions()),
    ]),
  component: UsersSettingsPage,
  pendingComponent: UsersSettingsPending,
})

function UsersSettingsPage() {
  const { data: users } = useSuspenseQuery(userListQueryOptions())
  const ownEmail = useAuthUsername()
  // POST /api/v1/auth/users is AbilityRoot-gated (handleCreateUser's own
  // doc comment): a non-root viewer can still reach this AbilityRead
  // page, so the trigger only renders once we can see, from this same
  // already-loaded list, that the signed-in account is root. Same
  // client-side heuristic UserTable's own "is this my row" check uses,
  // backed by the same server-side enforcement either way.
  const isRoot = users.some(
    (u) => u.email === ownEmail && u.abilities.includes('root'),
  )

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <UsersIcon className="size-4" />
          </div>
          <div>
            <h1 className="text-lg font-semibold text-foreground">Users</h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Everyone with access to this platform, and what each account
              can do.
            </p>
          </div>
        </div>
        {isRoot ? <CreateUserDialog /> : null}
      </div>
      <UserTable users={users} />
    </div>
  )
}

// Route-level fallback for the loader's pending phase, matching
// UserTable's own 6-column shape so the skeleton doesn't jump when real
// rows swap in.
function UsersSettingsPending() {
  return (
    <div className="space-y-6">
      <h1 className="text-lg font-semibold text-foreground">Users</h1>
      <TableSkeleton columnCount={6} />
    </div>
  )
}
