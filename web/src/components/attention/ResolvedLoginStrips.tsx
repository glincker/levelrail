import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ChevronRightGlassIcon, InfoGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { formatAge } from '../../lib/format'
import {
  useDeviceActivity,
  useDismissAttentionItems,
  type DeviceActivityItem,
} from '../../queries/deviceAuth'
import { CliLine } from '../fixit/FixCli'
import {
  DEVICE_LOGIN_COMMAND,
  ResolvedLoginControls,
} from './ResolvedLoginControls'

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
// for this user, the audit log keeps the record. Past two strips one
// summary row carries dismiss-all so a flood never stacks up.
export function ResolvedLoginStrips() {
  const resolved = useResolvedLogins()
  if (resolved.length === 0) {
    return null
  }
  const shown = resolved.slice(0, MAX_STRIPS)
  return (
    <div className="shrink-0" data-testid="resolved-login-strips">
      {shown.map((item) => (
        <Strip key={item.item_key} item={item} />
      ))}
      {resolved.length > MAX_STRIPS ? <SummaryRow items={resolved} /> : null}
    </div>
  )
}

function SummaryRow({ items }: { items: DeviceActivityItem[] }) {
  const { t } = useTranslation('attention', { useSuspense: false })
  const dismissAll = useDismissAttentionItems()
  const [open, setOpen] = useState(false)
  return (
    <div className="border-b border-border bg-muted/40 px-4 py-1 text-sm">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <InfoGlassIcon
          aria-hidden="true"
          className="size-4 shrink-0 text-tone-info"
        />
        <p role="status" className="min-w-0 flex-1 font-medium">
          {t('deviceLogin.resolvedSummary', { count: items.length })}
        </p>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          aria-expanded={open}
          onClick={() => {
            setOpen((v) => !v)
          }}
        >
          <ChevronRightGlassIcon
            aria-hidden="true"
            className={open ? 'size-4 rotate-90' : 'size-4'}
          />
          {t('deviceLogin.howToFix')}
        </Button>
        <Button size="sm" variant="ghost" render={<Link to="/status" />}>
          {t('deviceLogin.viewStatus')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={dismissAll.isPending}
          onClick={() => {
            dismissAll.mutate(
              items.map((i) => i.item_key),
              {
                onSuccess: () => {
                  toast.add({
                    title: t('deviceLogin.dismissedAll', {
                      count: items.length,
                    }),
                    type: 'success',
                  })
                },
                onError: (error) => {
                  toast.add({ title: error.message, type: 'error' })
                },
              },
            )
          }}
        >
          {t('deviceLogin.dismissAll')}
        </Button>
      </div>
      {open ? (
        <div className="py-1">
          <CliLine
            step={{
              label: t('deviceLogin.startOverHint'),
              command: DEVICE_LOGIN_COMMAND,
            }}
          />
        </div>
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
      className="flex items-center gap-x-3 border-b border-border bg-muted/40 px-4 py-1 text-sm"
    >
      <InfoGlassIcon
        aria-hidden="true"
        className="size-4 shrink-0 text-tone-info"
      />
      <p className="min-w-0 flex-1 truncate">
        <span className="font-medium">{title}</span>
        <span className="text-xs text-muted-foreground"> {meta}</span>
      </p>
      <ResolvedLoginControls
        compact
        dismissKey={item.item_key}
        auditPath={item.audit_path}
      />
    </div>
  )
}
