import { Display } from '@/components/ui/typography'
import { useTranslation } from 'react-i18next'
import { ShieldCheckGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { formatDate, formatSize } from '../../lib/format'
import type { CoverageSummary as Summary } from '../../types/backupCoverage'
import { coverageHeadline } from './coverageHelpers'

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="truncate text-sm font-medium">{value}</dd>
    </div>
  )
}

/** CoverageSummary leads the page: how many resources are protected, and the one action that fixes the rest. */
export function CoverageSummary({
  summary,
  onProtect,
  showAction = true,
}: {
  summary: Summary
  onProtect?: () => void
  showAction?: boolean
}) {
  const { t } = useTranslation('backups')
  const headline = coverageHeadline(summary)
  return (
    <section
      className="space-y-4 rounded-lg border border-border bg-card p-4"
      aria-labelledby="coverage-summary-title"
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <Display as="h2" size="section" id="coverage-summary-title">
            {t('summary.protected', {
              protected: summary.protected,
              total: summary.total,
            })}
          </Display>
          <p className="text-sm text-muted-foreground">
            {t(`summary.headline.${headline}`, {
              count: summary.unprotected,
              failing: summary.failing,
            })}
          </p>
        </div>
        {showAction && summary.unprotected > 0 ? (
          <Button type="button" onClick={onProtect}>
            <ShieldCheckGlassIcon
              className="size-4.5 text-tone-success"
              aria-hidden="true"
            />
            {t('summary.protectEverything')}
          </Button>
        ) : null}
      </div>
      <dl className="grid grid-cols-1 gap-3 sm:grid-cols-3">
        <Stat
          label={t('summary.lastVerified')}
          value={
            summary.last_verified_restore
              ? formatDate(summary.last_verified_restore, '-')
              : t('health.never')
          }
        />
        <Stat
          label={t('summary.nextRun')}
          value={
            summary.next_run
              ? formatDate(summary.next_run, '-')
              : t('health.notScheduled')
          }
        />
        <Stat
          label={t('summary.storage')}
          value={formatSize(summary.storage_bytes)}
        />
      </dl>
    </section>
  )
}
