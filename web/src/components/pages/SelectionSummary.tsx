import { useTranslation } from 'react-i18next'
import { CircleCheckGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { formatSize } from '../../lib/format'
import type { DropSelection } from '../../lib/zipSite'
import { summarizeSelection } from './pageHelpers'

/** Pre-publish facts and warnings, derived only from what was dropped. */
export function SelectionSummary({
  selection,
}: Readonly<{ selection: DropSelection }>) {
  const { t } = useTranslation('pagesSites')
  const s = summarizeSelection(selection)
  const warnings: string[] = []
  if (s.hasIndex === false) warnings.push(t('summary.noIndex'))
  if (s.overLimit) warnings.push(t('summary.overLimit'))
  if (selection.kind === 'folder') warnings.push(t('summary.skipped'))
  return (
    <div className="space-y-1.5 text-sm">
      <ul className="flex flex-wrap items-center gap-x-4 gap-y-1">
        {s.files !== null ? (
          <li>{t('summary.files', { count: s.files })}</li>
        ) : (
          <li className="font-mono text-xs">{selection.label}</li>
        )}
        <li className="text-muted-foreground">{formatSize(s.bytes)}</li>
        {s.hasIndex ? (
          <li className="flex items-center gap-1 text-success">
            <CircleCheckGlassIcon className="size-4" aria-hidden="true" />
            {t('summary.index')}
          </li>
        ) : null}
        {s.redirects ? <li>{t('summary.redirects')}</li> : null}
        {s.headers ? <li>{t('summary.headers')}</li> : null}
      </ul>
      {warnings.map((w) => (
        <p
          key={w}
          className="flex items-start gap-1.5 text-xs text-muted-foreground"
        >
          <TriangleAlertGlassIcon
            className="mt-0.5 size-3.5 shrink-0"
            aria-hidden="true"
          />
          {w}
        </p>
      ))}
    </div>
  )
}
