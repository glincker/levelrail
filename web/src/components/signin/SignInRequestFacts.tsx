import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ClockCountdownIcon,
  DesktopIcon,
  GlobeIcon,
  MapPinIcon,
  PlugsIcon,
  TerminalWindowIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { formatAbsoluteTime, formatRelativeTime } from '../../lib/relativeTime'
import {
  formatCountdown,
  ipScope,
  parseDeviceLabel,
  secondsLeft,
  type DeviceKind,
} from '../../lib/signInRequest'
import type { SignInApprovalRequest } from '../../queries/signIn'
import { deviceTitle } from './signInRequestHooks'

const DEVICE_ICONS = {
  browser: GlobeIcon,
  cli: TerminalWindowIcon,
  mcp: PlugsIcon,
  unknown: DesktopIcon,
} satisfies Record<DeviceKind, typeof GlobeIcon>

function Fact({
  icon,
  label,
  children,
}: {
  icon: ReactNode
  label: string
  children: ReactNode
}) {
  return (
    <div className="flex min-w-0 items-start gap-2">
      <span aria-hidden="true" className="mt-0.5 text-muted-foreground">
        {icon}
      </span>
      <div className="min-w-0">
        <dt className="text-xs text-muted-foreground">{label}</dt>
        <dd className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm font-medium wrap-anywhere">
          {children}
        </dd>
      </div>
    </div>
  )
}

export function SignInRequestFacts({
  request,
  now,
  compact,
}: {
  request: SignInApprovalRequest
  now: number
  compact: boolean
}) {
  const { t } = useTranslation('signIn', { useSuspense: false })
  const info = parseDeviceLabel(request.user_agent)
  const DeviceIcon = DEVICE_ICONS[info.kind]
  const scope = ipScope(request.requester_ip)
  const left = secondsLeft(request.expires_at, now)
  return (
    <dl
      className={
        compact
          ? 'grid grid-cols-1 gap-2'
          : 'grid grid-cols-1 gap-x-6 gap-y-3 sm:grid-cols-2'
      }
    >
      <Fact
        icon={<DeviceIcon className="size-4" />}
        label={t('request.factDevice')}
      >
        {deviceTitle(request.user_agent, t)}
      </Fact>
      <Fact
        icon={<MapPinIcon className="size-4" />}
        label={t('request.factWhere')}
      >
        <span className="font-mono">{request.requester_ip}</span>
        {scope === 'public' ? null : (
          <Badge variant="muted">{t(`request.scope.${scope}`)}</Badge>
        )}
      </Fact>
      <Fact
        icon={<ClockCountdownIcon className="size-4" />}
        label={t('request.factWhen')}
      >
        <time
          dateTime={request.created_at}
          title={formatAbsoluteTime(request.created_at)}
        >
          {formatRelativeTime(request.created_at, now)}
        </time>
      </Fact>
      {compact ? null : (
        <Fact
          icon={<ClockCountdownIcon className="size-4" />}
          label={t('request.factExpires')}
        >
          <span className="tabular-nums" aria-hidden="true">
            {formatCountdown(left)}
          </span>
          <span className="sr-only" role="status">
            {t('request.expiresInMinutes', { count: Math.ceil(left / 60) })}
          </span>
        </Fact>
      )}
    </dl>
  )
}
