import { LayersGlassIcon } from '@thesvg/ui-react'
import { useAppRollout } from '../../queries/appRollout'
import { useDeployDetailT } from './useDeployDetailT'

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div className="flex items-baseline gap-1.5">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="text-sm font-medium tabular-nums text-foreground">
        {value}
      </dd>
    </div>
  )
}

/** RolloutCountsPanel shows rolling rollout wave and replica counts; hidden otherwise. */
export function RolloutCountsPanel({ appName }: { appName: string }) {
  const { t } = useDeployDetailT()
  const { data } = useAppRollout(appName)
  const p = data?.progress
  if (!data || data.strategy !== 'rolling' || !p) return null
  const policy = data.policy
  return (
    <section
      aria-label={t('rollout.counts.title')}
      className="flex flex-wrap items-center gap-x-4 gap-y-1 rounded-lg border border-border px-3 py-2"
    >
      <span className="flex items-center gap-1.5 text-xs font-medium text-foreground">
        <LayersGlassIcon
          className="size-5 text-muted-foreground"
          aria-hidden="true"
        />
        {t('rollout.counts.wave', { wave: p.wave, waves: p.waves })}
      </span>
      <dl className="flex flex-wrap items-center gap-x-4 gap-y-1">
        <Stat label={t('rollout.counts.ready')} value={p.ready} />
        <Stat label={t('rollout.counts.updated')} value={p.updated} />
        <Stat label={t('rollout.counts.unavailable')} value={p.unavailable} />
        <Stat label={t('rollout.counts.surge')} value={p.surge} />
      </dl>
      {policy ? (
        <span className="ml-auto text-xs text-muted-foreground">
          {t('rollout.counts.policy', {
            surge: policy.max_surge ?? '-',
            unavailable: policy.max_unavailable ?? '-',
            min: policy.min_available ?? '-',
          })}
        </span>
      ) : null}
    </section>
  )
}
