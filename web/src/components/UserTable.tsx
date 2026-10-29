import { useTranslation } from 'react-i18next'
import { UserGlassIcon } from '@thesvg/ui-react'
import { Badge } from './ui/badge'
import { Button } from './ui/button'
import { EmptyState } from './ui/empty-state'
import { ResponsiveTable, type ResponsiveColumn } from './ui/responsive-table'
import { RemoveUserDialog } from './RemoveUserDialog'
import { RequirePasswordChangeDialog } from './RequirePasswordChangeDialog'
import { EditUserAbilitiesDialog } from './EditUserAbilitiesDialog'
import { UserRoleDialog } from './access/UserRoleDialog'
import { useExperimentalFeatures } from '../hooks/useExperimental'
import { useIsRoot } from '../hooks/useIsRoot'
import { isFeatureVisible } from '../lib/experimental'
import { useAuthUsername } from '../hooks/useAuthUsername'
import { ABILITY_BADGE_VARIANT } from '../types/token'
import type { UserResource } from '../queries/users'

export function UserTable({ users }: { users: UserResource[] }) {
  const { t } = useTranslation('settings')
  // The client-side mirror of "who am I signed in as" (authStore's own
  // doc comment): good enough to decide whether to show this row's edit
  // control, since PUT .../abilities enforces the real self-lockout rule
  // server-side regardless of what this heuristic gets wrong.
  const ownEmail = useAuthUsername()
  const isRoot = useIsRoot()
  const features = useExperimentalFeatures()
  const canAssignRoles = isRoot && isFeatureVisible('access-roles', features)

  if (users.length === 0) {
    return (
      <EmptyState
        icon={<UserGlassIcon className="size-5.5" />}
        title={t('usersPage.empty.title')}
        description={t('usersPage.empty.body')}
      />
    )
  }

  function authMethods(user: UserResource): string {
    const methods = [...user.providers]
    if (user.has_password) {
      methods.push(t('usersPage.table.password'))
    }
    return methods.length > 0
      ? methods.join(', ')
      : t('usersPage.table.noMethod')
  }

  const columns: ResponsiveColumn<UserResource>[] = [
    {
      id: 'email',
      primary: true,
      header: t('usersPage.table.email'),
      cell: (user) => (
        <span className="font-medium break-all text-foreground">
          {user.email}
          {user.is_first_user ? (
            <Badge variant="outline" className="ml-2">
              {t('usersPage.table.firstUser')}
            </Badge>
          ) : null}
          {ownEmail !== null && ownEmail === user.email ? (
            <Badge variant="muted" className="ml-2">
              {t('usersPage.table.you')}
            </Badge>
          ) : null}
          {user.disabled ? (
            <Badge variant="destructive" className="ml-2">
              {t('usersPage.table.disabled')}
            </Badge>
          ) : null}
          {user.must_change_password ? (
            <Badge variant="outline" className="ml-2">
              {t('usersPage.table.mustChangePassword')}
            </Badge>
          ) : null}
        </span>
      ),
    },
    {
      id: 'displayName',
      header: t('usersPage.table.displayName'),
      priority: 3,
      cell: (user) => user.display_name,
    },
    {
      id: 'signsInWith',
      header: t('usersPage.table.signsInWith'),
      priority: 3,
      cell: (user) => authMethods(user),
    },
    {
      id: 'role',
      header: t('usersPage.table.role'),
      priority: 2,
      cell: (user) =>
        user.role ? (
          <Badge variant={user.role === 'admin' ? 'destructive' : 'outline'}>
            {user.role.charAt(0).toUpperCase() + user.role.slice(1)}
          </Badge>
        ) : (
          <Badge variant="muted">{t('usersPage.table.custom')}</Badge>
        ),
    },
    {
      id: 'abilities',
      header: t('usersPage.table.abilities'),
      priority: 2,
      cell: (user) => (
        <div className="flex flex-wrap gap-1">
          {user.abilities.map((ability) => (
            <Badge key={ability} variant={ABILITY_BADGE_VARIANT[ability]}>
              {ability}
            </Badge>
          ))}
        </div>
      ),
    },
    {
      id: 'lastLogin',
      header: t('usersPage.table.lastLogin'),
      priority: 3,
      cell: (user) =>
        user.last_login_at
          ? new Date(user.last_login_at).toLocaleString()
          : t('usersPage.table.never'),
    },
  ]

  return (
    <ResponsiveTable
      rows={users}
      columns={columns}
      getRowId={(user) => user.id}
      ariaLabel={t('usersPage.table.label')}
      renderActions={(user) => {
        const isSelf = ownEmail !== null && ownEmail === user.email
        return (
          <div className="flex flex-wrap items-center justify-end gap-2">
            {isSelf ? (
              <Button
                variant="outline"
                size="sm"
                disabled
                title={t('usersPage.table.selfEditHint')}
              >
                {t('usersPage.table.editAbilities')}
              </Button>
            ) : (
              <>
                {canAssignRoles ? <UserRoleDialog user={user} /> : null}
                <EditUserAbilitiesDialog user={user} />
                {isRoot && (user.has_password || user.must_change_password) ? (
                  <RequirePasswordChangeDialog user={user} />
                ) : null}
              </>
            )}
            <RemoveUserDialog user={user} />
          </div>
        )
      }}
    />
  )
}
