import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

interface UptimeRow {
  component: string
  target: string
  uptime_percent: number | null
  probes: number
  summary: string
}

interface UptimeReport {
  month: string
  rows: UptimeRow[]
}

function currentMonth(): string {
  return new Date().toISOString().slice(0, 7)
}

// Monthly availability for this app, taken from the status page probes. It
// only appears once the app is a status page component.
export function AppUptimeReport({ app }: { app: string }) {
  const { t } = useTranslation('updates')
  const [month, setMonth] = useState(currentMonth)
  const report = useQuery({
    queryKey: ['status-page', 'uptime-report', month],
    queryFn: async (): Promise<UptimeReport> => {
      const res = await fetch(
        `/api/v1/status-page/uptime-report?month=${encodeURIComponent(month)}`,
      )
      if (!res.ok) throw new Error(`uptime report: ${res.status}`)
      return (await res.json()) as UptimeReport
    },
    retry: false,
  })
  const row = report.data?.rows.find((r) => r.target === app)
  return (
    <section className="space-y-2 rounded-lg border border-border bg-card p-4">
      <div className="flex items-center justify-between gap-3">
        <h3 className="text-sm font-medium">{t('uptimeReport.title')}</h3>
        <input
          type="month"
          value={month}
          max={currentMonth()}
          aria-label={t('uptimeReport.month')}
          onChange={(e) => e.target.value && setMonth(e.target.value)}
          className="rounded-md border border-input bg-background px-2 py-1 text-sm"
        />
      </div>
      {row && row.uptime_percent !== null ? (
        <>
          <p className="text-2xl font-semibold">
            {row.uptime_percent.toFixed(2)}%
          </p>
          <p className="select-all text-sm text-muted-foreground">
            {row.summary}
          </p>
        </>
      ) : (
        <p className="text-sm text-muted-foreground">
          {report.isPending
            ? t('uptimeReport.loading')
            : t('uptimeReport.none')}
        </p>
      )}
    </section>
  )
}
