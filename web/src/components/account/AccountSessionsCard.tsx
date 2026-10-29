import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { LogOutGlassIcon, MonitorGlassIcon } from '@thesvg/ui-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { ConfirmAction } from '@/components/ui/confirm-action'
import { EmptyState } from '@/components/ui/empty-state'
import { RelativeTime } from '../kit/RelativeTime'
import { CardError, CardLoading } from './AccountCardStates'
import { toast } from '@/components/ui/toast'
import {
  securitySessionsQueryOptions,
  useRevokeOtherSessions,
  useRevokeSession,
} from '../../queries/securityCenter'
import { useRevokeTrustedDevice } from '../../queries/signIn'
import { SessionDeviceRow } from './SessionDeviceRow'

const SELF = ''

export function AccountSessionsCard() {
  const { t } = useTranslation('settings')
  const { data, isPending, isError, isFetching, refetch } = useQuery(
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
        {isPending ? <CardLoading rows={3} /> : null}
        {isError ? (
          <CardError
            retrying={isFetching}
            onRetry={() => {
              void refetch()
            }}
          />
        ) : null}
        {data ? (
          <>
            <NetworkSummary sessions={data.sessions} />
            {data.sessions.length === 0 ? (
              <EmptyState
                compact
                icon={<MonitorGlassIcon className="size-5.5" />}
                title={t('accountPage.sessions.empty')}
                description={t('accountPage.sessions.emptyBody')}
              />
            ) : (
              <ul className="divide-y divide-border text-sm">
                {data.sessions.map((s) => (
                  <SessionDeviceRow
                    key={s.id}
                    session={s}
                    now={now}
                    busy={revoke.isPending && revoke.variables.id === s.id}
                    onRevoke={(done) => {
                      revoke.mutate(
                        { id: s.id, userId: SELF },
                        {
                          onSuccess: () => {
                            done()
                            toast.add({
                              title: t('accountPage.sessions.revoked'),
                              type: 'success',
                            })
                          },
                          onError,
                        },
                      )
                    }}
                  />
                ))}
              </ul>
            )}
            <ConfirmAction
              trigger={(open) => (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={revokeOthers.isPending || data.sessions.length < 2}
                  onClick={open}
                >
                  <LogOutGlassIcon size="1.2em" aria-hidden="true" />
                  {t('accountPage.sessions.revokeOthers')}
                </Button>
              )}
              title={t('accountPage.sessions.revokeOthersConfirmTitle')}
              consequences={[
                t('accountPage.sessions.revokeOthersConsequence'),
                t('accountPage.sessions.revokeOthersKeep'),
              ]}
              confirmLabel={t('accountPage.sessions.revokeOthers')}
              pending={revokeOthers.isPending}
              onConfirm={(done) => {
                revokeOthers.mutate(SELF, {
                  onSuccess: (res) => {
                    done()
                    toast.add({
                      title: t('accountPage.sessions.revokedOthers', {
                        count: res.revoked,
                      }),
                      type: 'success',
                    })
                  },
                  onError,
                })
              }}
            />
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
                          {d.ip}
                          {d.last_used_at ? (
                            <>
                              {', '}
                              <RelativeTime at={d.last_used_at} />
                            </>
                          ) : null}
                        </p>
                      </div>
                      <ConfirmAction
                        trigger={(open) => {
                          const busy =
                            removeDevice.isPending &&
                            removeDevice.variables === d.id
                          return (
                            <Button
                              type="button"
                              size="sm"
                              variant="outline"
                              disabled={busy}
                              aria-label={t(
                                'accountPage.sessions.deviceRemoveLabel',
                                { device: d.label },
                              )}
                              onClick={open}
                            >
                              {busy
                                ? t('accountPage.sessions.deviceRemoving')
                                : t('accountPage.sessions.deviceRemove')}
                            </Button>
                          )
                        }}
                        title={t('accountPage.sessions.deviceConfirmTitle', {
                          device: d.label,
                        })}
                        consequences={[
                          t('accountPage.sessions.deviceConsequence'),
                        ]}
                        confirmLabel={t('accountPage.sessions.deviceRemove')}
                        pending={
                          removeDevice.isPending &&
                          removeDevice.variables === d.id
                        }
                        onConfirm={(done) => {
                          removeDevice.mutate(d.id, {
                            onSuccess: () => {
                              done()
                              toast.add({
                                title: t('accountPage.sessions.deviceRemoved'),
                                type: 'success',
                              })
                            },
                            onError,
                          })
                        }}
                      />
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
