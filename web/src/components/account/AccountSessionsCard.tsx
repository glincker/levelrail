import { useQuery } from '@tanstack/react-query'
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
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import {
  securitySessionsQueryOptions,
  useRevokeOtherSessions,
  useRevokeSession,
} from '../../queries/securityCenter'
import { useRevokeTrustedDevice } from '../../queries/signIn'

const SELF = ''

function when(iso: string): string {
  return iso ? new Date(iso).toLocaleString() : ''
}

export function AccountSessionsCard() {
  const { t } = useTranslation('settings')
  const { data, isPending, isError } = useQuery(
    securitySessionsQueryOptions(SELF),
  )
  const revoke = useRevokeSession()
  const revokeOthers = useRevokeOtherSessions()
  const removeDevice = useRevokeTrustedDevice()
  const onError = (e: Error) => toast.add({ title: e.message, type: 'error' })

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('accountPage.sessions.title')}</CardTitle>
        <CardDescription>
          {t('accountPage.sessions.description')}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        {isPending ? (
          <p className="text-sm text-muted-foreground">
            {t('accountPage.loading')}
          </p>
        ) : null}
        {isError ? (
          <p className="text-sm text-destructive">
            {t('accountPage.loadError')}
          </p>
        ) : null}
        {data ? (
          <>
            <ul className="divide-y divide-border text-sm">
              {data.sessions.map((s) => (
                <li key={s.id} className="flex items-center gap-3 py-2">
                  <DesktopIcon className="size-4 shrink-0" aria-hidden="true" />
                  <div className="min-w-0 flex-1">
                    <p className="truncate font-medium">
                      {s.browser}
                      {s.current ? (
                        <Badge variant="success" className="ml-2">
                          {t('accountPage.sessions.current')}
                        </Badge>
                      ) : null}
                    </p>
                    <p className="text-xs text-muted-foreground">
                      {s.network}, {t('accountPage.sessions.lastSeen')}{' '}
                      {when(s.last_seen_at)}
                    </p>
                  </div>
                  {s.current ? null : (
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      disabled={revoke.isPending}
                      onClick={() =>
                        revoke.mutate(
                          { id: s.id, userId: SELF },
                          {
                            onSuccess: () =>
                              toast.add({
                                title: t('accountPage.sessions.revoked'),
                                type: 'success',
                              }),
                            onError,
                          },
                        )
                      }
                    >
                      {t('accountPage.sessions.revoke')}
                    </Button>
                  )}
                </li>
              ))}
            </ul>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={revokeOthers.isPending || data.sessions.length < 2}
              onClick={() =>
                revokeOthers.mutate(SELF, {
                  onSuccess: (res) =>
                    toast.add({
                      title: t('accountPage.sessions.revokedOthers', {
                        count: res.revoked,
                      }),
                      type: 'success',
                    }),
                  onError,
                })
              }
            >
              <SignOutIcon aria-hidden="true" />
              {t('accountPage.sessions.revokeOthers')}
            </Button>
            <div className="space-y-2">
              <h3 className="text-sm font-medium">
                {t('accountPage.sessions.devices')}
              </h3>
              {data.trusted_devices.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  {t('accountPage.sessions.devicesEmpty')}
                </p>
              ) : (
                <ul className="divide-y divide-border text-sm">
                  {data.trusted_devices.map((d) => (
                    <li key={d.id} className="flex items-center gap-3 py-2">
                      <div className="min-w-0 flex-1">
                        <p className="truncate font-medium">{d.label}</p>
                        <p className="text-xs text-muted-foreground">
                          {d.ip}, {when(d.last_used_at)}
                        </p>
                      </div>
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        disabled={removeDevice.isPending}
                        onClick={() =>
                          removeDevice.mutate(d.id, {
                            onSuccess: () =>
                              toast.add({
                                title: t('accountPage.sessions.deviceRemoved'),
                                type: 'success',
                              }),
                            onError,
                          })
                        }
                      >
                        {t('accountPage.sessions.deviceRemove')}
                      </Button>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </>
        ) : null}
      </CardContent>
    </Card>
  )
}
