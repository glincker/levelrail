import { useTranslation } from 'react-i18next'
import { WarningCircleIcon } from '@phosphor-icons/react/dist/ssr'
import { EmptyState } from '@/components/ui/empty-state'
import { SkeletonList, StatusPill } from '@/components/kit'
import { useDeployEventComparison } from '../../queries/deployEvents'
import { formatMs, verdictTone, codeKey } from '../../lib/deployEvents'
import type {
  DeployEventPhaseDelta,
  DeployEventTimeline,
} from '../../types/deployEvents'
import { EventList } from './EventList'

function Delta({ d }: { d: DeployEventPhaseDelta }) {
  const { t } = useTranslation('deployEvents')
  if (d.delta_ms === undefined) return <span>{t('compare.missing')}</span>
  if (d.delta_ms === 0) return <span>{t('compare.same')}</span>
  const value = formatMs(Math.abs(d.delta_ms))
  return (
    <span
      className={d.delta_ms > 0 ? 'text-tone-warning' : 'text-tone-success'}
    >
      {d.delta_ms > 0
        ? t('compare.slower', { value })
        : t('compare.faster', { value })}
    </span>
  )
}

function Column({
  title,
  timeline,
}: {
  title: string
  timeline: DeployEventTimeline
}) {
  const { t } = useTranslation('deployEvents')
  return (
    <section
      aria-label={t('compare.columnLabel', { id: timeline.deploy_id })}
      className="min-w-0 rounded-xl border p-4"
    >
      <header className="mb-3 flex flex-wrap items-center gap-2">
        <h3 className="text-sm font-medium">{title}</h3>
        <span className="font-mono text-xs text-muted-foreground">
          {timeline.deploy_id}
        </span>
        <StatusPill
          size="sm"
          tone={verdictTone(timeline.verdict)}
          label={t(`verdict.${timeline.verdict}`)}
        />
      </header>
      <EventList
        events={timeline.events}
        showLogs={false}
        label={t('compare.columnLabel', { id: timeline.deploy_id })}
      />
    </section>
  )
}

function CodeChips({ label, codes }: { label: string; codes: string[] }) {
  const { t } = useTranslation('deployEvents')
  if (codes.length === 0) return null
  return (
    <div className="flex flex-wrap items-center gap-2 text-sm">
      <span className="text-muted-foreground">{label}</span>
      {codes.map((c) => (
        <code key={c} className="rounded bg-muted px-1.5 py-0.5 text-xs">
          {t(`codes.${codeKey(c)}` as 'codes.push', { defaultValue: c })}
        </code>
      ))}
    </div>
  )
}

export function CompareView({ id, other }: { id: string; other: string }) {
  const { t } = useTranslation('deployEvents')
  const q = useDeployEventComparison(id, other)
  if (q.isPending) {
    return (
      <div role="status" aria-label={t('compare.loading')}>
        <SkeletonList rows={4} />
      </div>
    )
  }
  if (q.isError) {
    return (
      <EmptyState
        icon={<WarningCircleIcon />}
        title={t('compare.errorTitle')}
        description={q.error.message}
      />
    )
  }
  const c = q.data
  return (
    <div className="flex flex-col gap-4">
      <section
        aria-label={t('compare.title')}
        className="rounded-xl border p-4"
      >
        <table className="w-full text-sm">
          <caption className="sr-only">{t('compare.title')}</caption>
          <thead>
            <tr className="text-left text-muted-foreground">
              <th scope="col" className="pb-2 font-medium">
                {t('compare.phase')}
              </th>
              <th scope="col" className="pb-2 font-medium">
                {t('compare.deployA')}
              </th>
              <th scope="col" className="pb-2 font-medium">
                {t('compare.deployB')}
              </th>
              <th scope="col" className="pb-2 font-medium">
                {t('compare.delta')}
              </th>
            </tr>
          </thead>
          <tbody>
            {c.phases.map((p) => (
              <tr key={p.phase} className="border-t">
                <th scope="row" className="py-1.5 text-left font-normal">
                  {t(`phases.${p.phase}` as 'phases.build', {
                    defaultValue: p.phase,
                  })}
                </th>
                <td className="py-1.5 tabular-nums">
                  {p.a_ms === undefined ? '-' : formatMs(p.a_ms)}
                </td>
                <td className="py-1.5 tabular-nums">
                  {p.b_ms === undefined ? '-' : formatMs(p.b_ms)}
                </td>
                <td className="py-1.5 tabular-nums">
                  <Delta d={p} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <div className="mt-3 flex flex-col gap-2">
          <CodeChips label={t('compare.onlyHere')} codes={c.only_a} />
          <CodeChips label={t('compare.onlyThere')} codes={c.only_b} />
          {c.only_a.length === 0 && c.only_b.length === 0 && (
            <p className="text-sm text-muted-foreground">
              {t('compare.noDifferences')}
            </p>
          )}
        </div>
      </section>
      <div className="grid gap-4 lg:grid-cols-2">
        <Column title={t('compare.deployA')} timeline={c.a} />
        <Column title={t('compare.deployB')} timeline={c.b} />
      </div>
    </div>
  )
}
