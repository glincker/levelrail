import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { MailGlassIcon, UsersGlassIcon } from '@thesvg/ui-react'
import { userListQueryOptions } from '../../queries/users'
import { roleListQueryOptions } from '../../queries/roles'
import { inviteListQueryOptions } from '../../queries/invites'
import { UserTable } from '../../components/UserTable'
import { CreateUserDialog } from '../../components/CreateUserDialog'
import { InviteMemberDialog } from '../../components/InviteMemberDialog'
import { InvitesTable } from '../../components/InvitesTable'
import { useIsRoot } from '../../hooks/useIsRoot'
import { useAuthUsername } from '../../hooks/useAuthUsername'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Badge } from '@/components/ui/badge'
import { Banner } from '@/components/ui/banner'
import { useTranslation } from 'react-i18next'
import { hasAbility } from '../../types/token'
import type { CreateInviteResponse } from '../../queries/invites'
import { SettingsPageTitle } from '../../components/settings/SettingsPageTitle'

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
  const { t } = useTranslation('settings')
  const { data: users } = useSuspenseQuery(userListQueryOptions())
  const { data: invites } = useSuspenseQuery(inviteListQueryOptions())
  // Known accept links for invites created during this page session
  // (InvitesTable's own doc comment explains why this can't just be
  // reloaded from the server: only a token's hash is ever persisted).
  const [inviteLinks, setInviteLinks] = useState<Record<string, string>>({})
  // POST /api/v1/auth/users is AbilityRoot-gated (handleCreateUser's own
  // doc comment): a non-root viewer can still reach this AbilityRead
  // page, so the trigger only renders once useIsRoot (hooks/useIsRoot.ts)
  // can see, from this same already-loaded list, that the signed-in
  // account is root. Backed by the same server-side enforcement either
  // way. POST /api/v1/invites shares that exact gate (handleCreateInvite's
  // own doc comment), so the invite trigger follows the same check.
  const isRoot = useIsRoot()
  const ownEmail = useAuthUsername()
  const ownUser = users.find((u) => u.email === ownEmail)
  const ownAbilities = ownUser?.abilities ?? []
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

  const pendingInvites = invites.filter((inv) => !inv.expired).length

  return (
    <PageTemplate
      icon={<UsersGlassIcon size="1.4em" />}
      eyebrow={t('usersPage.eyebrow')}
      title={t('usersPage.title')}
      description={t('usersPage.description')}
      helpPath="/teams-and-roles"
      helpLabel={t('usersPage.help')}
      status={
        <Badge variant="muted">
          {t('usersPage.count', { count: users.length })}
        </Badge>
      }
      actions={
        canInvite ? (
          <>
            <InviteMemberDialog
              callerAbilities={ownAbilities}
              onCreated={handleInviteCreated}
            />
            {isRoot ? <CreateUserDialog /> : null}
          </>
        ) : null
      }
      banners={
        canInvite && pendingInvites > 0 ? (
          <Banner
            variant="info"
            title={t('usersPage.banner.title', { count: pendingInvites })}
          >
            {t('usersPage.banner.body')}
          </Banner>
        ) : null
      }
    >
      <UserTable users={users} />

      {canInvite ? (
        <div className="space-y-3">
          <div className="flex items-center gap-2">
            <MailGlassIcon className="size-5 text-muted-foreground" />
            <h2 className="text-sm font-semibold text-foreground">
              {t('usersPage.pendingInvites')}
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
    </PageTemplate>
  )
}

// Route-level fallback for the loader's pending phase, matching
// UserTable's own 6-column shape so the skeleton doesn't jump when real
// rows swap in.
function UsersSettingsPending() {
  return (
    <div className="space-y-6">
      <SettingsPageTitle id="users" />
      <TableSkeleton columnCount={6} />
    </div>
  )
}
