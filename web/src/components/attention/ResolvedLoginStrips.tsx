import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { InfoIcon } from '@phosphor-icons/react/dist/ssr'
import { formatAge } from '../../lib/format'
import {
  useDeviceActivity,
  type DeviceActivityItem,
} from '../../queries/deviceAuth'
import { ResolvedLoginControls } from './ResolvedLoginControls'

const MAX_STRIPS = 2

// Expired and denied logins, newest first, that this user has not dismissed.
function useResolvedLogins(): DeviceActivityItem[] {
  const { data } = useDeviceActivity()
  return (data ?? [])
    .filter(
      (i) => !i.dismissed && (i.state === 'expired' || i.state === 'denied'),
    )
    .sort((a, b) => b.expires_at.localeCompare(a.expires_at))
}

// Informational, dismissable strips under the waiting-login banner. Unlike
// a waiting login they never block anything: dismissing hides the strip
// for this user, the audit log keeps the record.
export function ResolvedLoginStrips() {
  const { t } = useTranslation('attention', { useSuspense: false })
  const resolved = useResolvedLogins()
  if (resolved.length === 0) {
    return null
  }
  const shown = resolved.slice(0, MAX_STRIPS)
  const more = resolved.length - shown.length
  return (
    <div className="shrink-0" data-testid="resolved-login-strips">
      {shown.map((item) => (
        <Strip key={item.item_key} item={item} />
      ))}
      {more > 0 ? (
        <p className="border-b border-border bg-muted/40 px-4 py-1 text-xs text-muted-foreground">
          <Link to="/status" className="underline">
            {t('deviceLogin.resolvedMore', { count: more })}
          </Link>
        </p>
      ) : null}
    </div>
  )
}

function Strip({ item }: { item: DeviceActivityItem }) {
  const { t } = useTranslation('attention', { useSuspense: false })
  const name =
    item.client_name ||
    item.user_agent.split(' ')[0] ||
    t('deviceLogin.unknownDevice')
  const title =
    item.state === 'expired'
      ? t('deviceLogin.resolvedExpired', { name })
      : t('deviceLogin.resolvedDenied', { name })
  const meta = [item.requester_ip, formatAge(item.expires_at)]
    .filter(Boolean)
    .join(', ')

  return (
    <div
      role="status"
      className="flex flex-wrap items-center gap-x-4 gap-y-2 border-b border-border bg-muted/40 px-4 py-2 text-sm"
    >
      <InfoIcon aria-hidden="true" className="size-5 shrink-0" />
      <div className="min-w-0 flex-1">
        <p className="font-medium">{title}</p>
        <p className="text-xs text-muted-foreground">{meta}</p>
      </div>
      <ResolvedLoginControls
        dismissKey={item.item_key}
        auditPath={item.audit_path}
      />
    </div>
  )
}
