import { useDeployDetailT } from '../deployDetail/useDeployDetailT'
import type { CacheChipState } from '../../lib/buildStages'

const TONE: Record<Exclude<CacheChipState['kind'], 'none'>, string> = {
  hit: 'border-tone-success-border bg-tone-success-soft text-tone-success',
  partial: 'border-tone-warning-border bg-tone-warning-soft text-tone-warning',
  miss: 'border-border bg-muted text-muted-foreground',
}

/** Cache result of one stage: hit, partial with N of M, or miss. */
export function CacheChip({ state }: { state: CacheChipState }) {
  const { t } = useDeployDetailT()
  if (state.kind === 'none') return null
  const label =
    state.kind === 'partial'
      ? t('buildStages.cache.partial', {
          cached: state.cached,
          total: state.total,
        })
      : t(`buildStages.cache.${state.kind}`)
  return (
    <span
      data-testid="cache-chip"
      data-kind={state.kind}
      className={`inline-flex items-center rounded border px-1.5 py-0.5 text-xs ${TONE[state.kind]}`}
    >
      {label}
    </span>
  )
}
