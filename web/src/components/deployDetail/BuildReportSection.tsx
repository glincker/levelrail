import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { ApiError } from '../../lib/apiError'
import {
  useBuildReport,
  type BuildReportRoute,
  type RouteKind,
} from '../../queries/nextReport'

const KIND_VARIANT: Record<
  RouteKind,
  'success' | 'info' | 'warning' | 'muted'
> = { static: 'success', isr: 'info', dynamic: 'warning', unknown: 'muted' }

// Twelve fixed steps so the bar width stays a Tailwind class, not a style.
const BAR_STEPS = [
  'w-0',
  'w-1/12',
  'w-2/12',
  'w-3/12',
  'w-4/12',
  'w-5/12',
  'w-6/12',
  'w-7/12',
  'w-8/12',
  'w-9/12',
  'w-10/12',
  'w-11/12',
  'w-full',
]

function barClass(value: number, max: number): string {
  if (max <= 0) return 'w-0'
  const step = Math.min(12, Math.max(1, Math.round((value / max) * 12)))
  return BAR_STEPS[step] ?? 'w-0'
}

function kb(n: number): string {
  return `${n.toLocaleString(undefined, { maximumFractionDigits: 1 })} kB`
}

export function BuildReportSection({
  appName,
  deployId,
}: {
  appName: string
  deployId: string
}) {
  const { t } = useTranslation('deployDetail', { keyPrefix: 'buildReport' })
  const q = useBuildReport(appName, deployId)

  if (q.error instanceof ApiError && q.error.status === 501) {
    return (
      <section aria-label={t('title')} className="space-y-2">
        <h2 className="text-sm font-medium">{t('title')}</h2>
        <p className="text-sm text-muted-foreground">{t('notStored')}</p>
      </section>
    )
  }
  const data = q.data
  if (!data || !data.detected) return null

  const routes: BuildReportRoute[] = [...data.routes].sort(
    (a, b) => b.first_load_kb - a.first_load_kb,
  )
  const hasSizes = routes.some((r) => r.size_kb > 0 || r.first_load_kb > 0)
  const max = routes.reduce((m, r) => Math.max(m, r.first_load_kb), 0)

  return (
    <section aria-label={t('title')} className="space-y-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-sm font-medium">{t('title')}</h2>
        {data.shared_first_load_kb != null && hasSizes ? (
          <span className="text-xs text-muted-foreground">
            {t('shared')}: {kb(data.shared_first_load_kb)}
          </span>
        ) : null}
      </div>
      {routes.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('noRoutes')}</p>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-border">
          <table className="w-full text-sm">
            <thead className="bg-muted/40 text-left text-xs text-muted-foreground">
              <tr>
                <th scope="col" className="px-3 py-1.5 font-medium">
                  {t('route')}
                </th>
                <th scope="col" className="px-3 py-1.5 font-medium">
                  {t('kind')}
                </th>
                {hasSizes ? (
                  <>
                    <th
                      scope="col"
                      className="px-3 py-1.5 text-right font-medium"
                    >
                      {t('size')}
                    </th>
                    <th scope="col" className="px-3 py-1.5 font-medium">
                      {t('firstLoad')}
                    </th>
                  </>
                ) : null}
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {routes.map((r) => (
                <tr key={r.path}>
                  <td className="px-3 py-1.5 font-mono text-xs">{r.path}</td>
                  <td className="px-3 py-1.5">
                    <Badge variant={KIND_VARIANT[r.kind]}>
                      {t(`kinds.${r.kind}`)}
                    </Badge>
                  </td>
                  {hasSizes ? (
                    <>
                      <td className="px-3 py-1.5 text-right tabular-nums">
                        {kb(r.size_kb)}
                      </td>
                      <td className="px-3 py-1.5">
                        <div className="flex items-center gap-2">
                          <span
                            className="h-1.5 w-24 shrink-0 rounded-full bg-muted"
                            aria-hidden="true"
                          >
                            <span
                              className={`block h-full rounded-full bg-primary ${barClass(r.first_load_kb, max)}`}
                            />
                          </span>
                          <span className="tabular-nums">
                            {kb(r.first_load_kb)}
                          </span>
                        </div>
                      </td>
                    </>
                  ) : null}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {data.warnings.length > 0 ? (
        <div
          role="status"
          className="rounded-lg border border-tone-warning-border bg-tone-warning-soft px-3 py-2 text-sm"
        >
          <p className="font-medium">{t('warnings')}</p>
          <ul className="list-disc space-y-0.5 pl-5">
            {data.warnings.map((w) => (
              <li key={w}>{w}</li>
            ))}
          </ul>
        </div>
      ) : null}
    </section>
  )
}
