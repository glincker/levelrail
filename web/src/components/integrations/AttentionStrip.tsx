import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ChevronRightGlassIcon } from '@thesvg/ui-react'
import { Banner } from '@/components/ui/banner'
import { StatusBadge } from '@/components/ui/status-badge'
import {
  expiryInfo,
  MANAGE_PATH,
  effectiveHealth,
  healthState,
} from '../../lib/connections'
import type { Connection } from '../../types/connections'

// Shown only when something needs action; an all-green hub stays quiet.
export function AttentionStrip({
  items,
  now,
}: Readonly<{ items: Connection[]; now: number }>) {
  const { t } = useTranslation('integrations')
  if (items.length === 0) return null
  return (
    <Banner
      variant="warning"
      title={t('attention.title', { count: items.length })}
    >
      <ul className="-mx-2 mt-1 divide-y divide-tone-warning-border">
        {items.map((c) => {
          const info = expiryInfo(c, now)
          const health = effectiveHealth(c, now)
          const reason =
            health === 'ok' && info.days !== null
              ? t('expiry.expiresInDays', { count: Math.max(info.days, 0) })
              : t(`health.${health}`)
          return (
            <li key={c.ref}>
              <Link
                to={MANAGE_PATH[c.kind]}
                className="group flex min-h-(--glinr-control-h) items-center gap-3 rounded-(--glinr-radius-sm) px-2 py-1.5 text-foreground transition-colors duration-(--glinr-motion-fast) hover:bg-background/60 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none motion-reduce:transition-none"
              >
                <span className="min-w-0 flex-1 truncate font-medium">
                  {c.label}
                </span>
                <StatusBadge
                  form="dot"
                  state={health === 'ok' ? 'degraded' : healthState(health)}
                  label={reason}
                />
                <ChevronRightGlassIcon
                  className="size-4 shrink-0 text-muted-foreground"
                  aria-hidden="true"
                />
              </Link>
            </li>
          )
        })}
      </ul>
    </Banner>
  )
}
