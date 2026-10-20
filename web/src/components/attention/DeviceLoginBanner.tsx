import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { TerminalWindowIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import {
  formatCountdown,
  liveDeviceRequests,
  msUntilExpiry,
  requesterLabel,
} from '../../lib/deviceLogin'
import {
  useDenyDeviceAuthRequest,
  useLiveDeviceAuthRequests,
  type DeviceAuthRequest,
} from '../../queries/deviceAuth'
import { DeviceLoginConfirmDialog } from './DeviceLoginConfirmDialog'
import { ResolvedLoginStrips } from './ResolvedLoginStrips'
import { useNow } from '../../hooks/useNow'

// Shown on every page while a CLI login waits. There is deliberately no
// dismiss button: it disappears only when the request is approved, denied
// or expires, so an operator cannot lose track of it. Logins that then
// expired or were denied stay below as dismissable informational strips.
export function DeviceLoginBanner() {
  const { data } = useLiveDeviceAuthRequests()
  const now = useNow()
  const live = liveDeviceRequests(data, now)
  const first = live.at(0)
  return (
    <>
      {first ? (
        <BannerBody request={first} others={live.length - 1} now={now} />
      ) : null}
      <ResolvedLoginStrips />
    </>
  )
}

function BannerBody({
  request,
  others,
  now,
}: {
  request: DeviceAuthRequest
  others: number
  now: number
}) {
  const { t } = useTranslation('attention', { useSuspense: false })
  const [confirming, setConfirming] = useState(false)
  const deny = useDenyDeviceAuthRequest()
  const name = requesterLabel(request) || t('deviceLogin.unknownDevice')

  return (
    <div
      role="alert"
      className="flex shrink-0 flex-wrap items-center gap-x-4 gap-y-2 border-b border-primary/30 bg-primary/10 px-4 py-2 text-sm"
    >
      <TerminalWindowIcon aria-hidden="true" className="size-5 shrink-0" />
      <div className="min-w-0 flex-1">
        <p className="font-medium">{t('deviceLogin.bannerTitle')}</p>
        <p className="text-xs text-muted-foreground">
          {t('deviceLogin.device', { name })}
          {request.requester_ip
            ? `, ${t('deviceLogin.from', { ip: request.requester_ip })}`
            : ''}
          {', '}
          {t('deviceLogin.expiresIn', {
            time: formatCountdown(msUntilExpiry(request, now)),
          })}
          {others > 0
            ? `. ${t('deviceLogin.bannerMore', { count: others })}`
            : ''}
        </p>
        {request.ip_mismatch ? (
          <p className="flex items-center gap-1 text-xs font-medium text-destructive">
            <WarningIcon aria-hidden="true" className="size-3.5" />
            {t('deviceLogin.ipMismatch', { ip: request.requester_ip })}
          </p>
        ) : null}
      </div>
      <span
        className="font-mono text-xl font-semibold tracking-widest"
        data-testid="device-banner-code"
      >
        {request.user_code}
      </span>
      <div className="flex items-center gap-2">
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={deny.isPending}
          onClick={() => {
            deny.mutate(request.user_code, {
              onSuccess: () => {
                toast.add({
                  title: t('deviceLogin.denied', { name }),
                  type: 'success',
                })
              },
              onError: (error) => {
                toast.add({ title: error.message, type: 'error' })
              },
            })
          }}
        >
          {t('deviceLogin.deny')}
        </Button>
        <Button
          type="button"
          size="sm"
          onClick={() => {
            setConfirming(true)
          }}
        >
          {t('deviceLogin.review')}
        </Button>
        <Button
          size="sm"
          variant="ghost"
          render={<Link to="/settings/cli-access" />}
        >
          {t('items.openCliAccess')}
        </Button>
      </div>
      <DeviceLoginConfirmDialog
        request={request}
        open={confirming}
        onOpenChange={setConfirming}
      />
    </div>
  )
}
