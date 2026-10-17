import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { spendCapHistoryQueryOptions } from '../../queries/spendCap'
import { RelativeTime } from '../kit/RelativeTime'

const KINDS = [
  'threshold',
  'enforced',
  'released',
  'rollover',
  'configured',
  'cleared',
] as const
type Kind = (typeof KINDS)[number]

function isKind(k: string): k is Kind {
  return (KINDS as readonly string[]).includes(k)
}

/** BudgetHistory lists what the budget did this and earlier months. */
export function BudgetHistory({ appName }: { appName: string }) {
  const { t } = useTranslation('budget')
  const { data, isError } = useQuery(spendCapHistoryQueryOptions(appName))
  return (
    <section aria-labelledby="budget-history-title" className="space-y-2">
      <h3 id="budget-history-title" className="text-sm font-medium">
        {t('history.title')}
      </h3>
      {isError ? (
        <p className="text-xs text-muted-foreground">
          {t('history.loadError')}
        </p>
      ) : data && data.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t('history.empty')}</p>
      ) : (
        <ul className="divide-y divide-border text-sm">
          {(data ?? []).map((e) => (
            <li
              key={e.id}
              className="flex flex-wrap items-baseline justify-between gap-x-4 py-1.5"
            >
              <span className="min-w-0">
                <span className="font-medium">
                  {isKind(e.kind) ? t(`history.kind.${e.kind}`) : e.kind}
                </span>
                <span className="ml-2 text-muted-foreground">{e.detail}</span>
              </span>
              <span className="text-xs text-muted-foreground">
                <RelativeTime at={e.at} />
              </span>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
