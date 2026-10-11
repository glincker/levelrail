import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { DesktopIcon, SignOutIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toast } from '@/components/ui/toast'
import { useIsRoot } from '../../hooks/useIsRoot'
import { userListQueryOptions } from '../../queries/users'
import { useRevokeTrustedDevice } from '../../queries/signIn'
import {
  type SecuritySessions,
  securitySessionsQueryOptions,
  useRevokeOtherSessions,
  useRevokeSession,
} from '../../queries/securityCenter'

const SELF = ''

function when(iso: string): string {
  return iso ? new Date(iso).toLocaleString() : ''
}

function AccountPicker({
  value,
  onChange,
}: {
  value: string
  onChange: (id: string) => void
}) {
  const { t } = useTranslation('security')
  const { data: users } = useQuery(userListQueryOptions())
  const label = (id: string) =>
    id === SELF
      ? t('sessions.self')
      : ((users ?? []).find((u) => u.id === id)?.email ?? id)
  return (
    <div className="flex items-center gap-2 text-sm">
      <span className="text-muted-foreground">{t('sessions.viewing')}</span>
      <Select value={value} onValueChange={(v) => onChange(v ?? SELF)}>
        <SelectTrigger className="w-64" aria-label={t('sessions.viewing')}>
          <SelectValue>{(v: string) => label(v)}</SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={SELF}>{t('sessions.self')}</SelectItem>
          {(users ?? []).map((u) => (
            <SelectItem key={u.id} value={u.id}>
              {u.email}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}

function SessionsTable({
  data,
  userId,
}: {
  data: SecuritySessions
  userId: string
}) {
  const { t } = useTranslation('security')
  const revoke = useRevokeSession()
  if (data.sessions.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">{t('sessions.empty')}</p>
    )
  }
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('sessions.browser')}</TableHead>
          <TableHead>{t('sessions.network')}</TableHead>
          <TableHead>{t('sessions.created')}</TableHead>
          <TableHead>{t('sessions.lastSeen')}</TableHead>
          <TableHead />
        </TableRow>
      </TableHeader>
      <TableBody>
        {data.sessions.map((s) => (
          <TableRow key={s.id}>
            <TableCell>
              <span className="flex items-center gap-2">
                <DesktopIcon className="size-4 shrink-0" aria-hidden="true" />
                {s.browser}
                {s.current ? (
                  <Badge variant="success">{t('sessions.current')}</Badge>
                ) : null}
              </span>
            </TableCell>
            <TableCell className="font-mono text-xs">{s.network}</TableCell>
            <TableCell className="text-xs">{when(s.created_at)}</TableCell>
            <TableCell className="text-xs">{when(s.last_seen_at)}</TableCell>
            <TableCell className="text-right">
              {s.current ? null : (
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  aria-label={t('sessions.revokeAria', { id: s.id })}
                  disabled={revoke.isPending}
                  onClick={() => {
                    revoke.mutate(
                      { id: s.id, userId },
                      {
                        onSuccess: () =>
                          toast.add({
                            title: t('sessions.revoked'),
                            type: 'success',
                          }),
                        onError: (e) =>
                          toast.add({
                            title: e.message || t('sessions.revokeError'),
                            type: 'error',
                          }),
                      },
                    )
                  }}
                >
                  {t('sessions.revoke')}
                </Button>
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

function RevokeOthersButton({
  userId,
  self,
}: {
  userId: string
  self: boolean
}) {
  const { t } = useTranslation('security')
  const [open, setOpen] = useState(false)
  const revokeOthers = useRevokeOtherSessions()
  const label = self
    ? t('sessions.revokeOthers')
    : t('sessions.revokeOthersAdmin')
  return (
    <>
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={() => setOpen(true)}
      >
        <SignOutIcon aria-hidden="true" />
        {label}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('sessions.confirmTitle')}</DialogTitle>
            <DialogDescription>{t('sessions.confirmBody')}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setOpen(false)}
            >
              {t('sessions.cancel')}
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={revokeOthers.isPending}
              onClick={() => {
                revokeOthers.mutate(userId, {
                  onSuccess: (res) => {
                    setOpen(false)
                    toast.add({
                      title: t('sessions.revokeOthersDone', {
                        count: res.revoked,
                      }),
                      type: 'success',
                    })
                  },
                  onError: (e) =>
                    toast.add({
                      title: e.message || t('sessions.revokeError'),
                      type: 'error',
                    }),
                })
              }}
            >
              {t('sessions.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

function DevicesAndTokens({ data }: { data: SecuritySessions }) {
  const { t } = useTranslation('security')
  const removeDevice = useRevokeTrustedDevice()
  const qc = useQueryClient()
  return (
    <div className="grid gap-6 lg:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>{t('sessions.devicesTitle')}</CardTitle>
          <CardDescription>{t('sessions.devicesDescription')}</CardDescription>
        </CardHeader>
        <CardContent>
          {data.trusted_devices.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              {t('sessions.devicesEmpty')}
            </p>
          ) : (
            <ul className="divide-y divide-border text-sm">
              {data.trusted_devices.map((d) => (
                <li key={d.id} className="flex items-center gap-3 py-2">
                  <div className="min-w-0 flex-1">
                    <p className="truncate font-medium">
                      {d.label}
                      {d.current ? (
                        <Badge variant="success" className="ml-2">
                          {t('sessions.current')}
                        </Badge>
                      ) : null}
                    </p>
                    <p className="text-xs text-muted-foreground">
                      {d.ip}, {when(d.last_used_at)}
                    </p>
                  </div>
                  {data.self ? (
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      disabled={removeDevice.isPending}
                      onClick={() => {
                        removeDevice.mutate(d.id, {
                          onSuccess: () => {
                            void qc.invalidateQueries({
                              queryKey: ['security'],
                            })
                            toast.add({
                              title: t('sessions.deviceRemoved'),
                              type: 'success',
                            })
                          },
                          onError: (e) =>
                            toast.add({ title: e.message, type: 'error' }),
                        })
                      }}
                    >
                      {t('sessions.deviceRemove')}
                    </Button>
                  ) : null}
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>{t('sessions.tokensTitle')}</CardTitle>
          <CardDescription>{t('sessions.tokensDescription')}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {data.tokens.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              {t('sessions.tokensEmpty')}
            </p>
          ) : (
            <ul className="divide-y divide-border text-sm">
              {data.tokens.map((tok) => (
                <li key={tok.id} className="py-2">
                  <p className="font-medium">{tok.name}</p>
                  <p className="text-xs text-muted-foreground">
                    {tok.abilities.join(', ')},{' '}
                    {tok.last_used_at
                      ? t('sessions.usedAgo', { time: when(tok.last_used_at) })
                      : t('sessions.neverUsed')}
                  </p>
                </li>
              ))}
            </ul>
          )}
          <Link
            to="/settings/tokens"
            className={buttonVariants({ variant: 'outline', size: 'sm' })}
          >
            {t('sessions.manageTokens')}
          </Link>
        </CardContent>
      </Card>
    </div>
  )
}

export function SessionsPanel() {
  const { t } = useTranslation('security')
  const isRoot = useIsRoot()
  const [userId, setUserId] = useState(SELF)
  const { data, error } = useQuery(securitySessionsQueryOptions(userId))
  return (
    <div className="space-y-6">
      <Card>
        <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-3">
          <div>
            <CardTitle>{t('sessions.title')}</CardTitle>
            <CardDescription>{t('sessions.description')}</CardDescription>
          </div>
          {data ? (
            <RevokeOthersButton userId={userId} self={data.self} />
          ) : null}
        </CardHeader>
        <CardContent className="space-y-4">
          {isRoot ? (
            <AccountPicker value={userId} onChange={setUserId} />
          ) : null}
          {error ? (
            <p className="text-sm text-destructive">{error.message}</p>
          ) : null}
          {data ? <SessionsTable data={data} userId={userId} /> : null}
        </CardContent>
      </Card>
      {data ? <DevicesAndTokens data={data} /> : null}
    </div>
  )
}
