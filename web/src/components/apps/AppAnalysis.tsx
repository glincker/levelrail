import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  formatSizingLimit,
  sortCauses,
  sortReportsNewest,
  summarizeLogGroups,
  usableMetrics,
  usableSizing,
} from '@/lib/analysisReports'
import { formatAbsoluteTime, formatRelativeTime } from '@/lib/relativeTime'
import {
  useAnalysisReport,
  useAnalysisReports,
} from '../../queries/analysisReports'
import type {
  AnalysisDigest,
  AnalysisReportBody,
  AnalysisReportSummary,
} from '../../types/analysisReport'

const CHIP = 'rounded-full px-2 py-0.5 text-xs'

function EvidenceIds({ ids }: { ids: string[] }) {
  if (ids.length === 0) return null
  return (
    <span className="flex flex-wrap gap-1">
      {ids.map((id) => (
        <code
          key={id}
          className="rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground"
        >
          {id}
        </code>
      ))}
    </span>
  )
}

const num = (n: number) => String(Math.round(n * 100) / 100)

function DigestEvidence({ digest }: { digest?: AnalysisDigest | null }) {
  const { t } = useTranslation('common')
  if (!digest) return null
  const metrics = usableMetrics(digest.metrics)
  const sizing = usableSizing(digest.sizing)
  const levels = summarizeLogGroups(digest.logs ?? [])
  const restarts = digest.restarts > 0
  if (
    metrics.length === 0 &&
    sizing.length === 0 &&
    levels.length === 0 &&
    !restarts
  )
    return null
  return (
    <section aria-label={t('appAnalysis.evidence.title')} className="space-y-1">
      <h3 className="text-xs font-medium uppercase text-muted-foreground">
        {t('appAnalysis.evidence.title')}
      </h3>
      {metrics.length > 0 && (
        <div className="overflow-x-auto">
          <table className="text-xs">
            <thead className="text-left text-muted-foreground">
              <tr>
                <th scope="col" className="pr-4 font-medium">
                  {t('appAnalysis.evidence.metric')}
                </th>
                <th scope="col" className="pr-4 font-medium">
                  p50
                </th>
                <th scope="col" className="pr-4 font-medium">
                  p95
                </th>
                <th scope="col" className="pr-4 font-medium">
                  max
                </th>
                <th scope="col" className="font-medium">
                  {t('appAnalysis.evidence.trend')}
                </th>
              </tr>
            </thead>
            <tbody>
              {metrics.map((m) => (
                <tr key={m.evidence}>
                  <th scope="row" className="pr-4 text-left font-medium">
                    {m.metric}
                  </th>
                  <td className="pr-4 tabular-nums">{num(m.p50)}</td>
                  <td className="pr-4 tabular-nums">{num(m.p95)}</td>
                  <td className="pr-4 tabular-nums">{num(m.max)}</td>
                  <td className="text-muted-foreground">
                    {m.trend === 'flat' || m.trend === 'none'
                      ? t(`appAnalysis.evidence.trends.${m.trend}`)
                      : `${t(`appAnalysis.evidence.trends.${m.trend}`, { defaultValue: m.trend })} ${num(m.trend_pct)}%`}
                    {m.anomaly_count > 0 &&
                      `, ${t('appAnalysis.evidence.anomalies', { count: m.anomaly_count })}`}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {(sizing.length > 0 || levels.length > 0 || restarts) && (
        <ul className="flex flex-wrap items-center gap-1.5">
          {sizing.map((s) => {
            const cur = formatSizingLimit(s.dimension, s.current_limit)
            const sug = formatSizingLimit(s.dimension, s.suggested_limit)
            const changes =
              s.data_sufficient && s.suggested_limit > 0 && sug !== cur
            return (
              <li
                key={s.dimension}
                className={`${CHIP} bg-muted text-muted-foreground`}
              >
                {t(`appAnalysis.evidence.dimension.${s.dimension}`, {
                  defaultValue: s.dimension,
                })}
                {': '}
                {changes
                  ? t('appAnalysis.evidence.sizingChange', {
                      current: cur,
                      suggested: s.suggested_text || sug,
                    })
                  : cur}
              </li>
            )
          })}
          {levels.map((l) => (
            <li
              key={l.level}
              className={`${CHIP} bg-muted text-muted-foreground`}
            >
              {t('appAnalysis.evidence.logLevel', {
                level: l.level,
                lines: l.lines,
                groups: l.groups,
              })}
            </li>
          ))}
          {restarts && (
            <li className={`${CHIP} bg-muted text-muted-foreground`}>
              {t('appAnalysis.evidence.restarts', { count: digest.restarts })}
            </li>
          )}
        </ul>
      )}
    </section>
  )
}

export function AnalysisReportBodyView({ body }: { body: AnalysisReportBody }) {
  const { t } = useTranslation('common')
  const timeline = body.timeline ?? []
  const causes = sortCauses(body.causes ?? [])
  const recs = body.recommendations ?? []
  return (
    <div className="space-y-3 px-3 py-2 text-sm">
      {timeline.length > 0 && (
        <section aria-label={t('appAnalysis.timeline')} className="space-y-1">
          <h3 className="text-xs font-medium uppercase text-muted-foreground">
            {t('appAnalysis.timeline')}
          </h3>
          <ul className="space-y-1">
            {timeline.map((e, i) => (
              <li key={`${e.at}-${e.ref}-${i}`} className="flex gap-x-2">
                <span
                  className="shrink-0 text-muted-foreground"
                  title={formatAbsoluteTime(e.at)}
                >
                  {formatRelativeTime(e.at)}
                </span>
                <span className="font-medium">{e.kind}</span>
                <span className="text-muted-foreground">
                  {e.detail || e.ref}
                </span>
              </li>
            ))}
          </ul>
        </section>
      )}
      {causes.length > 0 && (
        <section aria-label={t('appAnalysis.causes')} className="space-y-1">
          <h3 className="text-xs font-medium uppercase text-muted-foreground">
            {t('appAnalysis.causes')}
          </h3>
          <ol className="space-y-1.5">
            {causes.map((c) => (
              <li key={c.id} className="flex flex-wrap items-center gap-2">
                <span>{c.title}</span>
                <span className={`${CHIP} bg-primary/10 text-primary`}>
                  {t('appAnalysis.confidence', { level: c.confidence })}
                </span>
                <EvidenceIds ids={c.evidence ?? []} />
              </li>
            ))}
          </ol>
        </section>
      )}
      <DigestEvidence digest={body.digest} />
      {recs.length > 0 && (
        <section
          aria-label={t('appAnalysis.recommendations')}
          className="space-y-1"
        >
          <h3 className="text-xs font-medium uppercase text-muted-foreground">
            {t('appAnalysis.recommendations')}
          </h3>
          <ul className="space-y-1.5">
            {recs.map((r) => (
              <li key={r.id} className="flex flex-wrap items-center gap-2">
                <span>{r.text}</span>
                {r.monthly_saving_usd ? (
                  <span className={`${CHIP} bg-muted text-muted-foreground`}>
                    {t('appAnalysis.saving', {
                      amount: r.monthly_saving_usd.toFixed(2),
                    })}
                  </span>
                ) : null}
                <EvidenceIds ids={r.evidence ?? []} />
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  )
}

function ReportDetail({ id }: { id: string }) {
  const { t } = useTranslation('common')
  const { data, isPending, isError } = useAnalysisReport(id, true)
  if (isPending)
    return (
      <p className="px-3 py-2 text-sm text-muted-foreground">
        {t('appAnalysis.loading')}
      </p>
    )
  if (isError || !data)
    return (
      <p role="alert" className="px-3 py-2 text-sm text-muted-foreground">
        {t('appAnalysis.loadFailed')}
      </p>
    )
  return <AnalysisReportBodyView body={data.report} />
}

function ReportRow({ report }: { report: AnalysisReportSummary }) {
  const { t } = useTranslation('common')
  const [open, setOpen] = useState(false)
  const detailId = `app-analysis-detail-${report.id}`
  const insufficient = report.status === 'insufficient_data'
  return (
    <li>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2 text-sm">
        <span
          className={`${CHIP} ${insufficient ? 'bg-muted text-muted-foreground' : 'bg-primary/10 font-medium text-primary'}`}
        >
          {t(`appAnalysis.status.${report.status}`)}
        </span>
        <span className="min-w-0 flex-1 truncate" title={report.summary}>
          {report.summary}
        </span>
        <span
          className="text-muted-foreground"
          title={formatAbsoluteTime(report.created_at)}
        >
          {formatRelativeTime(report.created_at)}
        </span>
        <span className={`${CHIP} bg-muted text-muted-foreground`}>
          {report.trigger_kind}
        </span>
        <button
          type="button"
          aria-expanded={open}
          aria-controls={detailId}
          onClick={() => setOpen((v) => !v)}
          className="rounded text-xs font-medium text-primary hover:underline focus-visible:outline focus-visible:outline-2 focus-visible:outline-ring"
        >
          {open ? t('appAnalysis.hide') : t('appAnalysis.view')}
        </button>
      </div>
      {open && (
        <div id={detailId} className="border-t border-border">
          <ReportDetail id={report.id} />
        </div>
      )}
    </li>
  )
}

export function AppAnalysisView({
  reports,
}: {
  reports?: AnalysisReportSummary[]
}) {
  const { t } = useTranslation('common')
  if (!reports || reports.length === 0) return null
  const rows = sortReportsNewest(reports)
  return (
    <section aria-labelledby="app-analysis-title" className="space-y-2">
      <h2 id="app-analysis-title" className="text-sm font-medium">
        {t('appAnalysis.title')}
      </h2>
      <ul className="divide-y divide-border rounded-lg border border-border">
        {rows.map((r) => (
          <ReportRow key={r.id} report={r} />
        ))}
      </ul>
    </section>
  )
}

export function AppAnalysis({ appName }: { appName: string }) {
  const { data } = useAnalysisReports(appName)
  return <AppAnalysisView reports={data} />
}
