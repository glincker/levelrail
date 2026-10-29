import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { formatAbsolute } from '../../lib/dateFormat'
import type { StackDrift } from '../../types/stacks'

export function DriftBadge({
  drift,
}: Readonly<{ drift: StackDrift | undefined }>) {
  const { t } = useTranslation('stacks')
  if (drift?.status !== 'drifted') return null
  return (
    <Badge
      variant="warning"
      title={
        drift.checked_at
          ? t('drift.checkedAt', { when: formatAbsolute(drift.checked_at) })
          : undefined
      }
    >
      <TriangleAlertGlassIcon
        className="text-tone-warning"
        size="1.4em"
        aria-hidden="true"
      />
      {t('drift.badge', { count: drift.summary.length })}
    </Badge>
  )
}

// Expandable list of what differs from the last applied revision; paths only.
export function DriftSummary({
  drift,
}: Readonly<{ drift: StackDrift | undefined }>) {
  const { t } = useTranslation('stacks')
  if (drift?.status !== 'drifted') return null
  return (
    <details className="rounded-lg border border-border bg-card text-sm">
      <summary className="flex cursor-pointer flex-wrap items-center gap-x-2 px-3 py-1.5 font-medium">
        {t('drift.title', { count: drift.summary.length })}
        <span className="text-xs font-normal text-muted-foreground">
          {drift.revision
            ? t('drift.revision', { number: drift.revision })
            : null}
          {drift.revision && drift.checked_at ? ' / ' : null}
          {drift.checked_at
            ? t('drift.checkedAt', { when: formatAbsolute(drift.checked_at) })
            : null}
        </span>
      </summary>
      <ul className="divide-y divide-border border-t border-border">
        {drift.summary.map((item) => (
          <li
            key={`${item.kind}/${item.scope ?? ''}/${item.name}`}
            className="flex flex-wrap items-baseline gap-x-2 px-3 py-1.5"
          >
            <span className="font-mono text-xs">
              {item.scope ? `${item.scope}/` : ''}
              {item.kind}/{item.name}
            </span>
            <Badge variant="muted">{t(`drift.action.${item.action}`)}</Badge>
            {item.fields && item.fields.length > 0 ? (
              <span className="font-mono text-xs text-muted-foreground">
                {item.fields.join(', ')}
              </span>
            ) : null}
          </li>
        ))}
      </ul>
      <p className="border-t border-border px-3 py-1.5 text-xs text-muted-foreground">
        {t('drift.noValues')}
      </p>
    </details>
  )
}
