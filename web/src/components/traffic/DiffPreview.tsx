import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'

import { diffRows, type DiffValue } from '@/lib/trafficDiff'

export interface DiffPreviewProps {
  before: Readonly<Record<string, DiffValue>>
  after: Readonly<Record<string, DiffValue>>
  labels?: Readonly<Record<string, string>>
  className?: string
}

function show(value: DiffValue | undefined, none: string): string {
  if (value === undefined || value === null) return none
  return String(value)
}

/** Two column before and after table, changed rows marked with text. */
export function DiffPreview({
  before,
  after,
  labels,
  className,
}: Readonly<DiffPreviewProps>) {
  const { t } = useTranslation('traffic')
  const [showAll, setShowAll] = useState(false)
  const rows = diffRows(before, after)
  const changed = rows.filter((r) => r.changed)
  const unchanged = rows.length - changed.length
  const visible = showAll ? rows : changed
  const none = t('diff.none')

  if (rows.length === 0) {
    return <p className="text-sm text-muted-foreground">{t('diff.empty')}</p>
  }

  return (
    <div className={cn('space-y-2', className)}>
      {visible.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('diff.empty')}</p>
      ) : (
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="text-xs text-muted-foreground">
              <th scope="col" className="w-1/3 py-1 pr-3 font-medium">
                {t('diff.setting')}
              </th>
              <th scope="col" className="py-1 pr-3 font-medium">
                {t('diff.before')}
              </th>
              <th scope="col" className="py-1 font-medium">
                {t('diff.after')}
              </th>
            </tr>
          </thead>
          <tbody>
            {visible.map((row) => (
              <tr
                key={row.key}
                data-changed={row.changed}
                className={cn(
                  'border-t border-border align-top',
                  row.changed && 'bg-amber-100 dark:bg-amber-900/30',
                )}
              >
                <th scope="row" className="py-1.5 pr-3 font-medium">
                  {labels?.[row.key] ?? row.key}
                  {row.changed ? (
                    <span className="sr-only"> ({t('diff.changed')})</span>
                  ) : null}
                </th>
                <td className="py-1.5 pr-3 font-mono text-[13px] break-all">
                  {row.changed ? <span aria-hidden="true">- </span> : null}
                  {show(row.before, none)}
                </td>
                <td className="py-1.5 font-mono text-[13px] break-all">
                  {row.changed ? <span aria-hidden="true">+ </span> : null}
                  {show(row.after, none)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {unchanged > 0 ? (
        <button
          type="button"
          aria-expanded={showAll}
          onClick={() => setShowAll((v) => !v)}
          className="text-xs font-medium text-[var(--brand-accent)] underline-offset-2 hover:underline dark:text-[var(--brand-accent-dark)]"
        >
          {showAll
            ? t('diff.hideUnchanged')
            : t('diff.showUnchanged', { count: unchanged })}
        </button>
      ) : null}
    </div>
  )
}
