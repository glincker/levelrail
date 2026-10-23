import { useTranslation } from 'react-i18next'
import { idleMinutes, type FlowValues, type UrlPreview } from './logic'

const MINUTES_PER_HOUR = 60
const MINUTES_PER_DAY = 1440

export function useSummaryText(values: FlowValues, preview: UrlPreview) {
  const { t } = useTranslation('functions')
  const mins = idleMinutes(values)
  let sleep = t('deploy.summary.sleepMinutes', { count: mins })
  if (mins === MINUTES_PER_HOUR) sleep = t('deploy.summary.sleepHour')
  if (mins === MINUTES_PER_DAY) sleep = t('deploy.summary.sleepDay')
  const url =
    preview.routeUrl ?? preview.functionUrl ?? t('deploy.summary.urlPending')
  const build =
    values.source === 'image'
      ? t('deploy.summary.buildImage')
      : t('deploy.summary.buildRailpack')
  return t('deploy.summary.text', { build, sleep, url })
}
