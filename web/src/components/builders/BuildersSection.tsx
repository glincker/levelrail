import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { HammerGlassIcon } from '@thesvg/ui-react'
import { StatusBadge } from '@/components/ui/status-badge'
import { RelativeTime } from '@/components/kit'
import { InfoTip } from '../kit/InfoTip'
import { runnerState } from '../../lib/buildRunnerFormat'
import { useBuildRunners } from '../../queries/buildRunners'
import { AddBuilderButton } from './AddBuilderButton'
import { BuildTierSelect } from './BuildTierSelect'

// Build-capable nodes from the runners API; the control plane is not a builder to enroll.
export function BuildersSection() {
  const { t } = useTranslation('nodes')
  const { t: td } = useTranslation('deploys')
  const { data } = useBuildRunners()
  const builders = (data?.runners ?? []).filter((r) => r.kind === 'node')

  return (
    <section aria-labelledby="builders-title" className="space-y-2">
      <div className="flex flex-wrap items-center gap-2">
        <h2
          id="builders-title"
          className="flex items-center gap-2 text-sm font-semibold"
        >
          <HammerGlassIcon aria-hidden="true" className="size-4" />
          {t('builders.title')}
          <InfoTip label={t('builders.tipLabel')}>{t('builders.tip')}</InfoTip>
        </h2>
        <span className="ml-auto">
          <AddBuilderButton />
        </span>
      </div>
      {builders.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('builders.empty')}</p>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-border">
          <table className="w-full text-sm">
            <thead className="text-left text-xs text-muted-foreground">
              <tr className="border-b border-border">
                <th className="px-3 py-2 font-medium">
                  {t('builders.col.name')}
                </th>
                <th className="px-3 py-2 font-medium">
                  {t('builders.col.status')}
                </th>
                <th className="px-3 py-2 font-medium">
                  {t('builders.col.tier')}
                </th>
                <th className="px-3 py-2 font-medium">
                  {t('builders.col.arch')}
                </th>
                <th className="px-3 py-2 font-medium">
                  {t('builders.col.builds')}
                </th>
                <th className="px-3 py-2 font-medium">
                  {t('builders.col.lastSeen')}
                </th>
              </tr>
            </thead>
            <tbody>
              {builders.map((b) => (
                <tr
                  key={b.id}
                  className="border-b border-border last:border-b-0"
                >
                  <td className="px-3 py-2 font-medium">
                    <Link to="/nodes/$id" params={{ id: b.id }}>
                      {b.name}
                    </Link>
                  </td>
                  <td className="px-3 py-2">
                    <StatusBadge
                      state={runnerState(b.status)}
                      label={td(`runners.status.${b.status}`)}
                    />
                  </td>
                  <td className="px-3 py-2">
                    <BuildTierSelect
                      id={`builder-tier-${b.id}`}
                      nodeId={b.id}
                      value={b.trust ?? ''}
                      label={t('builders.tierFor', { name: b.name })}
                      className="w-32"
                    />
                  </td>
                  <td className="px-3 py-2 text-muted-foreground">
                    {b.architecture ?? '-'}
                  </td>
                  <td className="px-3 py-2 tabular-nums text-muted-foreground">
                    {t('builders.buildsNow', {
                      used: b.slots_used,
                      total: b.slots_total,
                    })}
                  </td>
                  <td className="px-3 py-2 text-muted-foreground">
                    {b.last_seen_at ? (
                      <RelativeTime at={b.last_seen_at} />
                    ) : (
                      t('builders.never')
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}
