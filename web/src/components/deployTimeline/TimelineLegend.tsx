import { useTranslation } from 'react-i18next'
import {
  BellRingGlassIcon,
  CircleXGlassIcon,
  UndoGlassIcon,
} from '@thesvg/ui-react'
import { BAR_CLASS, barTone } from '../../lib/deployTimeline'

const SWATCHES = [
  { status: 'ready', key: 'legend.ready' },
  { status: 'building', key: 'legend.building' },
  { status: 'failed', key: 'legend.failed' },
  { status: 'canceled', key: 'legend.canceled' },
] as const

export function TimelineLegend() {
  const { t } = useTranslation('deployTimeline')
  return (
    <ul
      aria-label={t('legend.label')}
      className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground"
    >
      {SWATCHES.map((s) => (
        <li key={s.status} className="flex items-center gap-1.5">
          <span
            aria-hidden="true"
            className={`size-2.5 rounded-sm ${BAR_CLASS[barTone({ status: s.status })]}`}
          />
          {t(s.key)}
        </li>
      ))}
      <li className="flex items-center gap-1.5">
        <UndoGlassIcon aria-hidden="true" className="size-4.5" />
        {t('legend.rolledBack')}
      </li>
      <li className="flex items-center gap-1.5">
        <CircleXGlassIcon
          aria-hidden="true"
          className="size-4.5 text-tone-danger"
        />
        {t('legend.failed')}
      </li>
      <li className="flex items-center gap-1.5">
        <BellRingGlassIcon
          aria-hidden="true"
          className="size-4.5 text-tone-warning"
        />
        {t('legend.alert')}
      </li>
      <li className="flex items-center gap-1.5">
        <span aria-hidden="true" className="h-3 w-px bg-tone-info-solid" />
        {t('legend.now')}
      </li>
    </ul>
  )
}
