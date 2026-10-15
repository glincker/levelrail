import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { SignOutIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import {
  securitySessionsQueryOptions,
  useRevokeOtherSessions,
  useRevokeSession,
} from '../../queries/securityCenter'
import { useRevokeTrustedDevice } from '../../queries/signIn'
import { SessionDeviceRow } from './SessionDeviceRow'

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
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = window.setInterval(() => {
      setNow(Date.now())
    }, 30_000)
    return () => {
      window.clearInterval(id)
    }
  }, [])
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
            <NetworkSummary sessions={data.sessions} />
            <ul className="divide-y divide-border text-sm">
              {data.sessions.map((s) => (
                <SessionDeviceRow
                  key={s.id}
                  session={s}
                  now={now}
                  busy={revoke.isPending}
                  onRevoke={() =>
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
                />
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

function NetworkSummary({
  sessions,
}: {
  sessions: { network: string; current: boolean }[]
}) {
  const { t } = useTranslation('settings')
  const counts = new Map<string, number>()
  for (const s of sessions) {
    counts.set(s.network, (counts.get(s.network) ?? 0) + 1)
  }
  if (counts.size === 0) {
    return null
  }
  return (
    <div className="space-y-1.5">
      <p className="text-xs font-medium text-muted-foreground">
        {t('accountPage.sessions.networks', { count: counts.size })}
      </p>
      <ul className="flex flex-wrap gap-1.5">
        {[...counts.entries()].map(([network, n]) => (
          <li
            key={network}
            className="rounded-full border border-border px-2.5 py-0.5 font-mono text-xs text-foreground"
          >
            {network} <span className="text-muted-foreground">x{n}</span>
          </li>
        ))}
      </ul>
    </div>
  )
}
