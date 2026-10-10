import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import {
  codeLoginSettingsQueryOptions,
  signInRequestsQueryOptions,
  trustedDevicesQueryOptions,
  useRevokeTrustedDevice,
  useUpdateCodeLoginSettings,
} from '../queries/signIn'
import { useIsRoot } from '../hooks/useIsRoot'
import { SignInRequestsList } from './attention/SignInRequestsBanner'

// Settings > Security: waiting sign-ins, trusted browsers and the
// auth.code_login setting.
export function SignInSecurityCards() {
  return (
    <>
      <SignInRequestsCard />
      <TrustedDevicesCard />
      <CodeLoginSettingsCard />
    </>
  )
}

function SignInRequestsCard() {
  const { t } = useTranslation('signIn')
  const { data } = useQuery(signInRequestsQueryOptions())
  return (
    <Card id="sign-in-requests">
      <CardHeader>
        <CardTitle>{t('requests.bannerTitle')}</CardTitle>
        <CardDescription>{t('requests.warning')}</CardDescription>
      </CardHeader>
      <CardContent>
        <SignInRequestsList
          codes={data?.codes ?? []}
          approvals={data?.approvals ?? []}
        />
      </CardContent>
    </Card>
  )
}

function TrustedDevicesCard() {
  const { t } = useTranslation('signIn')
  const { data } = useQuery(trustedDevicesQueryOptions())
  const revoke = useRevokeTrustedDevice()
  const devices = data ?? []
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('devices.title')}</CardTitle>
        <CardDescription>{t('devices.description')}</CardDescription>
      </CardHeader>
      <CardContent>
        {devices.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('devices.empty')}</p>
        ) : (
          <ul className="divide-y divide-border">
            {devices.map((d) => (
              <li
                key={d.id}
                className="flex flex-wrap items-center gap-x-4 gap-y-1 py-2 text-sm"
              >
                <div className="min-w-0 flex-1">
                  <p className="truncate font-medium">
                    {d.label}
                    {d.current ? (
                      <span className="ml-2 text-xs text-muted-foreground">
                        {t('devices.current')}
                      </span>
                    ) : null}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {t('devices.lastUsed', {
                      time: new Date(d.last_used_at).toLocaleString(),
                      ip: d.ip,
                    })}
                  </p>
                </div>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={revoke.isPending}
                  onClick={() => {
                    revoke.mutate(d.id, {
                      onSuccess: () =>
                        toast.add({
                          title: t('devices.revoked'),
                          type: 'success',
                        }),
                      onError: (e) =>
                        toast.add({
                          title: e.message || t('devices.revokeFailed'),
                          type: 'error',
                        }),
                    })
                  }}
                >
                  {t('devices.revoke')}
                </Button>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}

function CodeLoginSettingsCard() {
  const { t } = useTranslation('signIn')
  const isRoot = useIsRoot()
  const { data } = useQuery(codeLoginSettingsQueryOptions())
  const update = useUpdateCodeLoginSettings()
  if (!data) {
    return null
  }
  const save = (body: { admins?: boolean; others?: boolean }) => {
    update.mutate(body, {
      onSuccess: () =>
        toast.add({ title: t('settings.saved'), type: 'success' }),
      onError: (e) =>
        toast.add({
          title: e.message || t('settings.saveFailed'),
          type: 'error',
        }),
    })
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('settings.title')}</CardTitle>
        <CardDescription>{t('settings.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3 text-sm">
        <div className="flex items-center justify-between gap-4">
          <div>
            <p className="font-medium">{t('settings.admins')}</p>
            <p className="text-xs text-muted-foreground">
              {t('settings.adminsHint')}
            </p>
          </div>
          <Switch
            checked={data.admins}
            disabled={!isRoot || update.isPending}
            aria-label={t('settings.admins')}
            onCheckedChange={(next) => save({ admins: next })}
          />
        </div>
        <div className="flex items-center justify-between gap-4">
          <p className="font-medium">{t('settings.others')}</p>
          <Switch
            checked={data.others}
            disabled={!isRoot || update.isPending}
            aria-label={t('settings.others')}
            onCheckedChange={(next) => save({ others: next })}
          />
        </div>
        <p className="text-xs text-muted-foreground">
          {data.new_device_approval
            ? t('settings.approvalOn')
            : t('settings.approvalOff')}
        </p>
      </CardContent>
    </Card>
  )
}
