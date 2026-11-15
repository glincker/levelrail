import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { expiryInfo, MANAGE_PATH, effectiveHealth } from '../../lib/connections'
import type { Connection } from '../../types/connections'

// Shown only when something needs action; an all-green hub stays quiet.
export function AttentionStrip({
  items,
  now,
}: Readonly<{ items: Connection[]; now: number }>) {
  const { t } = useTranslation('integrations')
  if (items.length === 0) return null
  return (
    <section
      aria-labelledby="integrations-attention"
      className="rounded-lg border border-tone-warning-border bg-tone-warning-soft p-3"
    >
      <h2
        id="integrations-attention"
        className="flex items-center gap-2 text-sm font-semibold text-tone-warning"
      >
        <TriangleAlertGlassIcon className="size-4" aria-hidden="true" />
        {t('attention.title', { count: items.length })}
      </h2>
      <ul className="mt-2 flex gap-2 overflow-x-auto pb-1 sm:flex-wrap sm:overflow-visible">
        {items.map((c) => {
          const info = expiryInfo(c, now)
          const health = effectiveHealth(c, now)
          const reason =
            health === 'ok' && info.days !== null
              ? t('expiry.expiresInDays', { count: Math.max(info.days, 0) })
              : t(`health.${health}`)
          return (
            <li key={c.ref} className="shrink-0">
              <Link
                to={MANAGE_PATH[c.kind]}
                className="flex items-center gap-2 rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground transition-colors hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
              >
                <span className="font-medium">{c.label}</span>
                <span className="text-muted-foreground">{reason}</span>
              </Link>
            </li>
          )
        })}
      </ul>
    </section>
  )
}
