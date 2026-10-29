import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { CoinsGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { useUsageReport } from '../../queries/usage'
import { formatMoney } from '../../lib/usageFormat'

// This app's cost so far this month from the shared usage report, so the
// Resources tab and the Usage page cost one request between them. Renders
// nothing until prices exist, so an unconfigured instance shows no zeroes.
export function AppCostChip({ appName }: { appName: string }) {
  const { t } = useTranslation('usage')
  const { data } = useUsageReport('mtd')
  if (!data || !data.configured) return null
  const row = data.resources.find((g) => g.kind === 'app' && g.name === appName)
  if (!row) return null
  const share =
    data.forecast !== undefined && data.totals.total > 0
      ? (row.total / data.totals.total) * data.forecast
      : undefined
  return (
    <Link to="/usage" title={t('chip.open')} className="inline-flex w-fit">
      <Badge
        variant="outline"
        className="gap-1.5 py-1"
        data-testid="app-cost-chip"
      >
        <CoinsGlassIcon size="1.4em" />
        <span>
          {t('chip.label')}:{' '}
          {t('chip.value', { amount: formatMoney(row.total, data.currency) })}
          {share !== undefined
            ? `, ${t('chip.forecast', { amount: formatMoney(share, data.currency) })}`
            : ''}
        </span>
      </Badge>
    </Link>
  )
}
