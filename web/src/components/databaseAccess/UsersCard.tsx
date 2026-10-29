import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { UserPlusGlassIcon, UsersGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { toast } from '@/components/ui/toast'
import type {
  DatabaseCredential,
  DatabaseUser,
} from '../../types/databaseAccess'
import {
  useDatabaseUsers,
  useRotateDatabaseUser,
  useSetDatabaseUserLogin,
} from '../../queries/databaseAccess'
import { CreateUserDialog } from './CreateUserDialog'
import { DeleteUserDialog } from './DeleteUserDialog'

function userStatus(user: DatabaseUser): 'disabled' | 'expired' | 'active' {
  if (!user.can_login) return 'disabled'
  return user.expired ? 'expired' : 'active'
}

const STATUS_VARIANT = {
  active: 'success',
  expired: 'warning',
  disabled: 'muted',
} as const

function UsersTable({
  users,
  databaseName,
  onCredential,
  onDelete,
}: {
  users: DatabaseUser[]
  databaseName: string
  onCredential: (c: DatabaseCredential) => void
  onDelete: (name: string) => void
}) {
  const { t } = useTranslation('databaseAccess')
  const rotate = useRotateDatabaseUser(databaseName)
  const setLogin = useSetDatabaseUserLogin(databaseName)

  function fail(err: Error) {
    toast.add({
      title: t('users.failedToast'),
      description: err.message,
      type: 'error',
    })
  }

  function toggleLogin(user: DatabaseUser) {
    setLogin.mutate(
      { role: user.name, login: !user.can_login },
      {
        onSuccess: () =>
          toast.add({
            title: t(
              user.can_login ? 'users.disableToast' : 'users.enableToast',
              { name: user.name },
            ),
            type: 'success',
          }),
        onError: fail,
      },
    )
  }

  const columns = useMemo<ResponsiveColumn<DatabaseUser>[]>(
    () => [
      {
        id: 'name',
        header: t('users.columns.name'),
        primary: true,
        cell: (user) => (
          <div>
            <div className="font-mono text-xs">{user.name}</div>
            <div className="text-xs font-normal text-muted-foreground">
              {t(`users.kind.${user.kind}`)}
            </div>
          </div>
        ),
      },
      {
        id: 'access',
        header: t('users.columns.access'),
        priority: 2,
        cell: (user) => (
          <span className="text-xs">
            {user.preset ? t(`users.preset.${user.preset}`) : '-'}
          </span>
        ),
      },
      {
        id: 'status',
        header: t('users.columns.status'),
        cell: (user) => {
          const status = userStatus(user)
          return (
            <Badge variant={STATUS_VARIANT[status]}>
              {t(`users.status.${status}`)}
            </Badge>
          )
        },
      },
      {
        id: 'connections',
        header: t('users.columns.connections'),
        priority: 3,
        cell: (user) => (
          <span className="text-xs">
            {user.connections}
            <span className="text-muted-foreground">
              {' / '}
              {user.connection_limit > 0
                ? user.connection_limit
                : t('users.unlimited')}
            </span>
          </span>
        ),
      },
      {
        id: 'expires',
        header: t('users.columns.expires'),
        priority: 3,
        cell: (user) => (
          <span className="text-xs">
            {user.valid_until
              ? new Date(user.valid_until).toLocaleDateString()
              : t('users.never')}
          </span>
        ),
      },
    ],
    [t],
  )

  return (
    <ResponsiveTable
      rows={users}
      columns={columns}
      getRowId={(u) => u.name}
      ariaLabel={t('users.tableLabel')}
      renderActions={(user) =>
        user.protected ? (
          <span className="text-xs text-muted-foreground">
            {t('users.protectedNote')}
          </span>
        ) : (
          <div className="flex justify-end gap-1">
            <Button
              variant="ghost"
              size="sm"
              disabled={rotate.isPending}
              onClick={() =>
                rotate.mutate(user.name, {
                  onSuccess: (cred) => {
                    toast.add({
                      title: t('users.rotateToast'),
                      description: cred.restarted_apps?.length
                        ? t('users.rotateRestarted', {
                            apps: cred.restarted_apps.join(', '),
                          })
                        : undefined,
                      type: 'success',
                    })
                    onCredential(cred)
                  },
                  onError: fail,
                })
              }
            >
              {t('users.rotate')}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              disabled={setLogin.isPending}
              onClick={() => {
                toggleLogin(user)
              }}
            >
              {user.can_login ? t('users.disable') : t('users.enable')}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => onDelete(user.name)}
            >
              {t('users.delete')}
            </Button>
          </div>
        )
      }
    />
  )
}

/** UsersCard lists the database's roles and offers create, rotate, disable and delete. */
export function UsersCard({
  databaseName,
  onCredential,
}: {
  databaseName: string
  onCredential: (c: DatabaseCredential) => void
}) {
  const { t } = useTranslation('databaseAccess')
  const { data: users, isLoading, error } = useDatabaseUsers(databaseName)
  const [creating, setCreating] = useState(false)
  const [deleting, setDeleting] = useState<string | null>(null)
  const others = (users ?? []).filter((u) => u.kind !== 'system' || u.can_login)
  const onlyAdmin = others.every((u) => u.protected)

  return (
    <Card>
      <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <CardTitle className="flex items-center gap-1.5 text-sm">
            <UsersGlassIcon className="size-4.5" aria-hidden="true" />
            {t('users.title')}
          </CardTitle>
          <p className="text-xs text-muted-foreground">
            {t('users.description')}
          </p>
        </div>
        <Button size="sm" onClick={() => setCreating(true)}>
          <UserPlusGlassIcon size="1.2em" aria-hidden="true" />
          {t('users.create')}
        </Button>
      </CardHeader>
      <CardContent className="space-y-3">
        {isLoading ? (
          <p className="text-sm text-muted-foreground" aria-live="polite">
            ...
          </p>
        ) : error ? (
          <p className="text-sm text-destructive" role="alert">
            {t('access.loadFailed', { error: error.message })}
          </p>
        ) : (
          <>
            <UsersTable
              users={others}
              databaseName={databaseName}
              onCredential={onCredential}
              onDelete={setDeleting}
            />
            {onlyAdmin ? (
              <EmptyState
                icon={<UsersGlassIcon className="size-5.5" />}
                title={t('users.emptyTitle')}
                description={t('users.emptyBody')}
                action={
                  <Button size="sm" onClick={() => setCreating(true)}>
                    {t('users.create')}
                  </Button>
                }
              />
            ) : null}
          </>
        )}
      </CardContent>
      <CreateUserDialog
        databaseName={databaseName}
        open={creating}
        onOpenChange={setCreating}
        onCreated={onCredential}
      />
      <DeleteUserDialog
        databaseName={databaseName}
        name={deleting}
        onClose={() => setDeleting(null)}
      />
    </Card>
  )
}
