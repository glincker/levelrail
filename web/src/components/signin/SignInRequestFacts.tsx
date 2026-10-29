import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CountdownGlassIcon,
  GlobeGlassIcon,
  LocateGlassIcon,
  MonitorGlassIcon,
  PlugGlassIcon,
  TerminalGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
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
  browser: GlobeGlassIcon,
  cli: TerminalGlassIcon,
  mcp: PlugGlassIcon,
  unknown: MonitorGlassIcon,
} satisfies Record<DeviceKind, typeof GlobeGlassIcon>

function Fact({
  icon,
  label,
  inline,
  children,
}: {
  icon: ReactNode
  label: string
  inline: boolean
  children: ReactNode
}) {
  return (
    <div className="flex min-w-0 items-start gap-2">
      <span aria-hidden="true" className="mt-0.5 text-muted-foreground">
        {icon}
      </span>
      <div className="min-w-0">
        <dt className={inline ? 'sr-only' : 'text-xs text-muted-foreground'}>
          {label}
        </dt>
        <dd
          className={cn(
            'flex flex-wrap items-center gap-x-2 gap-y-1 font-medium wrap-anywhere',
            inline ? 'text-xs' : 'text-sm',
          )}
        >
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
          ? 'flex flex-wrap items-center gap-x-4 gap-y-1'
          : 'grid grid-cols-2 gap-x-4 gap-y-2 rounded-lg bg-muted/60 px-3 py-2 shadow-inner ring-1 ring-inset ring-foreground/5 sm:grid-cols-4'
      }
    >
      <Fact
        inline={compact}
        icon={<DeviceIcon className="size-4" />}
        label={t('request.factDevice')}
      >
        {deviceTitle(request.user_agent, t)}
      </Fact>
      <Fact
        inline={compact}
        icon={<LocateGlassIcon className="size-5" />}
        label={t('request.factWhere')}
      >
        <span className="font-mono">{request.requester_ip}</span>
        {scope === 'public' ? null : (
          <Badge variant="muted">{t(`request.scope.${scope}`)}</Badge>
        )}
      </Fact>
      <Fact
        inline={compact}
        icon={<CountdownGlassIcon className="size-5" />}
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
          inline={compact}
          icon={<CountdownGlassIcon className="size-5" />}
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
