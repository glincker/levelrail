import { useTranslation } from 'react-i18next'
import { CircleCheckGlassIcon, XGlassIcon } from '@thesvg/ui-react'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import type { SiteDiagnosis } from '../../queries/pagesSites'

// i18n treats dots as nesting, so a doctor code like dns.points_here becomes dns_points_here.
const labelKey = (code: string) => `domains.check.${code.replaceAll('.', '_')}`

/** Compact ok/fail checklist for a domain; the reason is the tooltip, not a paragraph. */
export default function DomainChecklist({
  rows,
}: Readonly<{ rows: SiteDiagnosis[] }>) {
  const { t } = useTranslation('pagesSites')
  if (rows.length === 0) return null
  return (
    <ul aria-label={t('domains.checks')} className="flex flex-wrap gap-1.5">
      {rows.map((r) => (
        <li key={r.code}>
          <Tooltip>
            <TooltipTrigger
              render={<span tabIndex={0} />}
              className="flex items-center gap-1 rounded-md border border-border px-1.5 py-0.5 text-xs"
            >
              {r.ok ? (
                <CircleCheckGlassIcon
                  className="size-3.5 text-success"
                  aria-hidden="true"
                />
              ) : (
                <XGlassIcon
                  className="size-3.5 text-destructive"
                  aria-hidden="true"
                />
              )}
              <span className={r.ok ? undefined : 'text-destructive'}>
                {t(labelKey(r.code), { defaultValue: r.code })}
              </span>
              <span className="sr-only">
                {r.ok ? t('domains.done') : t('domains.failed')}: {r.hint}
              </span>
            </TooltipTrigger>
            <TooltipContent>{r.hint}</TooltipContent>
          </Tooltip>
        </li>
      ))}
    </ul>
  )
}
