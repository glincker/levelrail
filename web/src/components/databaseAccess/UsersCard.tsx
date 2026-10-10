import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { UserPlusIcon, UsersIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
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
import { useWindowedRows } from '../../lib/useWindowedRows'
import { CreateUserDialog } from './CreateUserDialog'
import { DeleteUserDialog } from './DeleteUserDialog'

function UserRow({
  user,
  databaseName,
  onCredential,
  onDelete,
}: {
  user: DatabaseUser
  databaseName: string
  onCredential: (c: DatabaseCredential) => void
  onDelete: (name: string) => void
}) {
  const { t } = useTranslation('databaseAccess')
  const rotate = useRotateDatabaseUser(databaseName)
  const setLogin = useSetDatabaseUserLogin(databaseName)
  const status = !user.can_login
    ? 'disabled'
    : user.expired
      ? 'expired'
      : 'active'

  function fail(err: Error) {
    toast.add({
      title: t('users.failedToast'),
      description: err.message,
      type: 'error',
    })
  }

  function toggleLogin() {
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

  return (
    <TableRow>
      <TableCell>
        <div className="font-mono text-xs">{user.name}</div>
        <div className="text-xs text-muted-foreground">
          {t(`users.kind.${user.kind}`)}
        </div>
      </TableCell>
      <TableCell className="text-xs">
        {user.preset ? t(`users.preset.${user.preset}`) : '-'}
      </TableCell>
      <TableCell>
        <Badge
          variant={
            status === 'active'
              ? 'success'
              : status === 'expired'
                ? 'warning'
                : 'muted'
          }
        >
          {t(`users.status.${status}`)}
        </Badge>
      </TableCell>
      <TableCell className="text-xs">
        {user.connections}
        <span className="text-muted-foreground">
          {' / '}
          {user.connection_limit > 0
            ? user.connection_limit
            : t('users.unlimited')}
        </span>
      </TableCell>
      <TableCell className="text-xs">
        {user.valid_until
          ? new Date(user.valid_until).toLocaleDateString()
          : t('users.never')}
      </TableCell>
      <TableCell className="text-right">
        {user.protected ? (
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
              onClick={toggleLogin}
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
        )}
      </TableCell>
    </TableRow>
  )
}

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
  const { parentRef, windowed, indices, padTop, padBottom } = useWindowedRows(
    users.length,
  )

  return (
    <div
      ref={parentRef}
      className={windowed ? 'max-h-[560px] overflow-auto' : 'overflow-auto'}
    >
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('users.columns.name')}</TableHead>
            <TableHead>{t('users.columns.access')}</TableHead>
            <TableHead>{t('users.columns.status')}</TableHead>
            <TableHead>{t('users.columns.connections')}</TableHead>
            <TableHead>{t('users.columns.expires')}</TableHead>
            <TableHead className="text-right">
              {t('users.columns.actions')}
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {padTop > 0 ? (
            <TableRow aria-hidden="true">
              <TableCell colSpan={6} style={{ height: padTop }} />
            </TableRow>
          ) : null}
          {indices.map((i) => {
            const user = users[i]
            if (!user) return null
            return (
              <UserRow
                key={user.name}
                user={user}
                databaseName={databaseName}
                onCredential={onCredential}
                onDelete={onDelete}
              />
            )
          })}
          {padBottom > 0 ? (
            <TableRow aria-hidden="true">
              <TableCell colSpan={6} style={{ height: padBottom }} />
            </TableRow>
          ) : null}
        </TableBody>
      </Table>
    </div>
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
      <CardHeader className="flex flex-row items-start justify-between gap-3">
        <div className="space-y-1">
          <CardTitle className="flex items-center gap-1.5 text-sm">
            <UsersIcon className="size-4" aria-hidden="true" />
            {t('users.title')}
          </CardTitle>
          <p className="text-xs text-muted-foreground">
            {t('users.description')}
          </p>
        </div>
        <Button size="sm" onClick={() => setCreating(true)}>
          <UserPlusIcon aria-hidden="true" />
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
                icon={<UsersIcon className="size-5" />}
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
