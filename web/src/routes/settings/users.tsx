import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { EnvelopeIcon, UsersIcon } from '@phosphor-icons/react/dist/ssr'
import { userListQueryOptions } from '../../queries/users'
import { roleListQueryOptions } from '../../queries/roles'
import { inviteListQueryOptions } from '../../queries/invites'
import { UserTable } from '../../components/UserTable'
import { CreateUserDialog } from '../../components/CreateUserDialog'
import { InviteMemberDialog } from '../../components/InviteMemberDialog'
import { InvitesTable } from '../../components/InvitesTable'
import { useAuthUsername } from '../../hooks/useAuthUsername'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { hasAbility } from '../../types/token'
import type { CreateInviteResponse } from '../../queries/invites'

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
      queryClient.ensureQueryData(inviteListQueryOptions()),
    ]),
  component: UsersSettingsPage,
  pendingComponent: UsersSettingsPending,
})

function UsersSettingsPage() {
  const { data: users } = useSuspenseQuery(userListQueryOptions())
  const { data: invites } = useSuspenseQuery(inviteListQueryOptions())
  const ownEmail = useAuthUsername()
  // Known accept links for invites created during this page session
  // (InvitesTable's own doc comment explains why this can't just be
  // reloaded from the server: only a token's hash is ever persisted).
  const [inviteLinks, setInviteLinks] = useState<Record<string, string>>({})
  // POST /api/v1/auth/users is AbilityRoot-gated (handleCreateUser's own
  // doc comment): a non-root viewer can still reach this AbilityRead
  // page, so the trigger only renders once we can see, from this same
  // already-loaded list, that the signed-in account is root. Same
  // client-side heuristic UserTable's own "is this my row" check uses,
  // backed by the same server-side enforcement either way.
  const ownUser = users.find((u) => u.email === ownEmail)
  const ownAbilities = ownUser?.abilities ?? []
  const isRoot = ownAbilities.includes('root')
  // POST /api/v1/invites is AbilityWrite-gated, not AbilityRoot
  // (handleCreateInvite's own privilege cap does the real enforcement,
  // routes.go), so the invite trigger and pending-invites panel follow
  // 'write' here, not the stricter isRoot above. InviteMemberDialog's own
  // role picker further caps itself to roles ownAbilities can actually
  // grant (handleCreateInvite's server-side cap is still the real
  // boundary either way).
  const canInvite = hasAbility(ownAbilities, 'write')

  function handleInviteCreated(invite: CreateInviteResponse) {
    setInviteLinks((prev) => ({ ...prev, [invite.id]: invite.link }))
  }

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
              Everyone with access to this platform, and what each account can
              do.
            </p>
          </div>
        </div>
        {canInvite ? (
          <div className="flex items-center gap-2">
            {canInvite ? (
              <InviteMemberDialog
                callerAbilities={ownAbilities}
                onCreated={handleInviteCreated}
              />
            ) : null}
            {isRoot ? <CreateUserDialog /> : null}
          </div>
        ) : null}
      </div>
      <UserTable users={users} />

      {canInvite ? (
        <div className="space-y-3">
          <div className="flex items-center gap-2">
            <EnvelopeIcon className="size-4 text-muted-foreground" />
            <h2 className="text-sm font-semibold text-foreground">
              Pending invites
            </h2>
          </div>
          <InvitesTable
            invites={invites}
            links={inviteLinks}
            isRoot={isRoot}
            ownUserID={ownUser?.id}
          />
        </div>
      ) : null}
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
