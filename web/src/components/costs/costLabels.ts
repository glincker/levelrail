import type { TFunction } from 'i18next'
import type { CostExploration, CostSeries } from '../../types/costExplorer'

// Display name of a series: ids of projects resolve through the response
// labels, the synthetic keys get translated copy, everything else is raw.
export function seriesLabel(
  res: Pick<CostExploration, 'labels' | 'group_by'>,
  s: Pick<CostSeries, 'key'>,
  t: TFunction<'costs'>,
): string {
  if (s.key === 'other') return t('series.other')
  if (s.key === 'unassigned') return t('series.unassigned')
  if (s.key === 'untagged') return t('series.untagged')
  if (res.group_by === 'project') {
    return res.labels[`project:${s.key}`] ?? s.key
  }
  if (res.group_by === 'kind')
    return t(`kind.${s.key}`, { defaultValue: s.key })
  return s.key
}
