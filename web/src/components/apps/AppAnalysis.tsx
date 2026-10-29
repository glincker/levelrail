import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { latestReport, sortCauses } from '@/lib/analysisReports'
import { formatAbsoluteTime, formatRelativeTime } from '@/lib/relativeTime'
import {
  useAnalysisReport,
  useAnalysisReports,
} from '../../queries/analysisReports'
import type {
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

export function AppAnalysisView({
  reports,
}: {
  reports?: AnalysisReportSummary[]
}) {
  const { t } = useTranslation('common')
  const [open, setOpen] = useState(false)
  const latest = latestReport(reports)
  if (!latest) return null
  const insufficient = latest.status === 'insufficient_data'
  return (
    <section aria-labelledby="app-analysis-title" className="space-y-2">
      <h2 id="app-analysis-title" className="text-sm font-medium">
        {t('appAnalysis.title')}
      </h2>
      <div className="rounded-lg border border-border">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2 text-sm">
          <span
            className={`${CHIP} ${insufficient ? 'bg-muted text-muted-foreground' : 'bg-primary/10 font-medium text-primary'}`}
          >
            {t(`appAnalysis.status.${latest.status}`)}
          </span>
          <span className="min-w-0 flex-1 truncate" title={latest.summary}>
            {latest.summary}
          </span>
          <span
            className="text-muted-foreground"
            title={formatAbsoluteTime(latest.created_at)}
          >
            {formatRelativeTime(latest.created_at)}
          </span>
          <span className={`${CHIP} bg-muted text-muted-foreground`}>
            {latest.trigger_kind}
          </span>
          <button
            type="button"
            aria-expanded={open}
            aria-controls="app-analysis-detail"
            onClick={() => setOpen((v) => !v)}
            className="rounded text-xs font-medium text-primary hover:underline focus-visible:outline focus-visible:outline-2 focus-visible:outline-ring"
          >
            {open ? t('appAnalysis.hide') : t('appAnalysis.view')}
          </button>
        </div>
        {open && (
          <div id="app-analysis-detail" className="border-t border-border">
            <ReportDetail id={latest.id} />
          </div>
        )}
      </div>
    </section>
  )
}

export function AppAnalysis({ appName }: { appName: string }) {
  const { data } = useAnalysisReports(appName)
  return <AppAnalysisView reports={data} />
}
