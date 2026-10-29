import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { CoinsGlassIcon } from '@thesvg/ui-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { formatMoney } from '@/lib/usageFormat'
import { useUsageReport } from '@/queries/usage'

const TOP = 5

// Hidden when cost is unavailable (no prices set, or the caller cannot read the report).
export function CostByTag({ onPickTag }: { onPickTag: (tag: string) => void }) {
  const { t } = useTranslation('resources')
  const report = useUsageReport('mtd')
  if (!report.data) return null
  const rows = (report.data.tags ?? []).slice(0, TOP)
  const max = Math.max(...rows.map((r) => r.total), 0)
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-sm">
          <CoinsGlassIcon size="1.4em" />
          {t('cost.title')}
        </CardTitle>
        <CardDescription>{t('cost.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-2">
        {rows.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('cost.empty')}</p>
        ) : (
          <ul className="space-y-2">
            {rows.map((r) => {
              const untagged = r.key === 'untagged'
              const label = untagged ? t('cost.untagged') : r.key
              return (
                <li key={r.key} className="space-y-1">
                  <div className="flex items-center justify-between gap-2 text-sm">
                    {untagged ? (
                      <span className="truncate text-muted-foreground">
                        {label}
                      </span>
                    ) : (
                      <button
                        type="button"
                        className="truncate text-left font-medium hover:underline"
                        onClick={() => onPickTag(r.key)}
                      >
                        {label}
                      </button>
                    )}
                    <span className="tabular-nums">
                      {formatMoney(r.total, report.data.currency)}
                    </span>
                  </div>
                  <progress
                    className="block h-1.5 w-full overflow-hidden rounded-full [&::-moz-progress-bar]:bg-primary [&::-webkit-progress-bar]:bg-muted [&::-webkit-progress-value]:bg-primary"
                    value={max > 0 ? r.total : 0}
                    max={max || 1}
                    aria-label={label}
                  />
                </li>
              )
            })}
          </ul>
        )}
        <Link to="/usage" className="text-xs text-primary hover:underline">
          {t('cost.open')}
        </Link>
      </CardContent>
    </Card>
  )
}
