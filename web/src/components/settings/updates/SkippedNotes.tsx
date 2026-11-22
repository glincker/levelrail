import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { selfUpgradePlanQueryOptions } from '../../../queries/selfUpgrade'

// Lists every release between the running version and the newest one, with
// breaking changes first so nothing is missed when several versions are skipped.
export function SkippedNotes() {
  const { t } = useTranslation('updates')
  const plan = useQuery(selfUpgradePlanQueryOptions(''))
  if (plan.isPending) {
    return (
      <p className="text-sm text-muted-foreground">{t('skipped.loading')}</p>
    )
  }
  const skipped = plan.data?.skipped ?? []
  if (plan.isError || skipped.length === 0) {
    return <p className="text-sm text-muted-foreground">{t('skipped.none')}</p>
  }
  return (
    <ol className="space-y-3">
      {skipped.map((r) => (
        <li key={r.version} className="rounded-md border border-border p-3">
          <p className="text-sm font-medium">{r.version}</p>
          {r.breaking.length > 0 ? (
            <div className="mt-2">
              <p className="text-xs font-medium uppercase text-destructive">
                {t('skipped.breaking')}
              </p>
              <ul className="list-disc pl-5 text-sm">
                {r.breaking.map((b) => (
                  <li key={b.id}>{b.summary}</li>
                ))}
              </ul>
            </div>
          ) : null}
          {r.notes ? (
            <p className="mt-2 line-clamp-6 whitespace-pre-wrap text-sm text-muted-foreground">
              {r.notes}
            </p>
          ) : null}
        </li>
      ))}
    </ol>
  )
}
