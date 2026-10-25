import { useTranslation } from 'react-i18next'
import type { RepoLinkPrefill } from '../../lib/deployRepoLink'

/** Read-only list of what the link filled in, shown before the wizard starts. */
export function RepoLinkSummary({
  prefill,
  templateName,
}: {
  prefill: RepoLinkPrefill
  templateName?: string
}) {
  const { t } = useTranslation('newApp')
  const rows: [string, string, boolean][] = []
  if (prefill.repo) rows.push([t('deployPage.repo'), prefill.repo, true])
  if (prefill.repo)
    rows.push([
      t('deployPage.branch'),
      prefill.branch ?? t('deployPage.branchDefault'),
      Boolean(prefill.branch),
    ])
  if (prefill.template)
    rows.push([
      t('deployPage.template'),
      templateName ?? prefill.template,
      false,
    ])
  if (prefill.name) rows.push([t('deployPage.name'), prefill.name, true])
  if (prefill.port !== undefined)
    rows.push([t('deployPage.port'), String(prefill.port), true])
  return (
    <dl className="grid gap-x-4 gap-y-2 rounded-lg border p-4 text-sm sm:grid-cols-[10rem_minmax(0,1fr)]">
      {rows.map(([label, value, mono]) => (
        <div key={label} className="contents">
          <dt className="text-muted-foreground">{label}</dt>
          <dd className={mono ? 'font-mono break-all' : 'break-all'}>
            {value}
          </dd>
        </div>
      ))}
    </dl>
  )
}
