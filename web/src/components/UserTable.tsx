import { UserIcon } from '@phosphor-icons/react/dist/ssr'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { useTranslation } from 'react-i18next'
import { Badge } from './ui/badge'
import { Button } from './ui/button'
import { EmptyState } from './ui/empty-state'
import { RemoveUserDialog } from './RemoveUserDialog'
import { EditUserAbilitiesDialog } from './EditUserAbilitiesDialog'
import { UserRoleDialog } from './access/UserRoleDialog'
import { useExperimentalFeatures } from '../hooks/useExperimental'
import { useIsRoot } from '../hooks/useIsRoot'
import { isFeatureVisible } from '../lib/experimental'
import { useAuthUsername } from '../hooks/useAuthUsername'
import { ABILITY_BADGE_VARIANT } from '../types/token'
import type { UserResource } from '../queries/users'

function formatDate(iso: string | undefined, fallback: string): string {
  return iso ? new Date(iso).toLocaleString() : fallback
}

function authMethods(user: UserResource): string {
  const methods = [...user.providers]
  if (user.has_password) {
    methods.push('password')
  }
  return methods.length > 0 ? methods.join(', ') : 'none'
}

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
        icon={<UserIcon className="size-5" />}
        title="No users found"
        description="Invite a teammate or create a user from the actions above."
      />
    )
  }

  const columns: ResponsiveColumn<UserResource>[] = [
    {
      id: 'email',
      primary: true,
      header: 'Email',
      cell: (user) => (
        <span className="font-medium break-all text-foreground">
          {user.email}
          {user.is_first_user ? (
            <Badge variant="outline" className="ml-2">
              First user
            </Badge>
          ) : null}
          {ownEmail !== null && ownEmail === user.email ? (
            <Badge variant="muted" className="ml-2">
              You
            </Badge>
          ) : null}
        </span>
      ),
    },
    {
      id: 'displayName',
      header: 'Display name',
      priority: 3,
      cell: (user) => user.display_name,
    },
    {
      id: 'signsInWith',
      header: 'Signs in with',
      priority: 3,
      cell: (user) => authMethods(user),
    },
    {
      id: 'role',
      header: 'Role',
      priority: 2,
      cell: (user) =>
        user.role ? (
          <Badge variant={user.role === 'admin' ? 'destructive' : 'outline'}>
            {user.role.charAt(0).toUpperCase() + user.role.slice(1)}
          </Badge>
        ) : (
          <Badge variant="muted">Custom</Badge>
        ),
    },
    {
      id: 'abilities',
      header: 'Abilities',
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
      header: 'Last login',
      priority: 3,
      cell: (user) => formatDate(user.last_login_at, 'Never'),
    },
  ]

  return (
    <ResponsiveTable
      rows={users}
      columns={columns}
      getRowId={(user) => user.id}
      ariaLabel={t('tables.users')}
      renderActions={(user) => {
        const isSelf = ownEmail !== null && ownEmail === user.email
        return (
          <div className="flex flex-wrap items-center justify-end gap-2">
            {isSelf ? (
              <Button
                variant="outline"
                size="sm"
                disabled
                title="You cannot edit your own abilities"
              >
                Edit abilities
              </Button>
            ) : (
              <>
                {canAssignRoles ? <UserRoleDialog user={user} /> : null}
                <EditUserAbilitiesDialog user={user} />
              </>
            )}
            <RemoveUserDialog user={user} />
          </div>
        )
      }}
    />
  )
}
