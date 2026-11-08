import type { ReactNode } from 'react'
import { RefreshCwGlassIcon } from '@thesvg/ui-react'
import { useTranslation } from 'react-i18next'
import { Button } from './ui/button'
import { SegmentedControl } from './ui/segmented-control'
import { TIME_RANGE_PRESETS, type TimeRangeKey } from '../lib/timeRange'

// Presentational only: each dashboard owns rangeKey and the refresh nonce.

// Short keys keep six segments compact; 'custom' leaves every segment unselected.
const OPTIONS = TIME_RANGE_PRESETS.map((p) => ({ value: p.key, label: p.key }))

export function TimeRangeControls({
  rangeKey,
  onRangeChange,
  onRefresh,
  children,
}: {
  /** 'custom' highlights no preset. */
  rangeKey: TimeRangeKey | 'custom'
  onRangeChange: (key: TimeRangeKey) => void
  onRefresh: () => void
  children?: ReactNode
}) {
  const { t } = useTranslation('uiShared')
  return (
    <div className="flex flex-wrap items-center gap-2">
      <SegmentedControl<TimeRangeKey | 'custom'>
        aria-label={t('timeRangeControls.label')}
        value={rangeKey}
        options={OPTIONS}
        onValueChange={(key) => {
          if (key !== 'custom') onRangeChange(key)
        }}
      />
      <Button type="button" variant="outline" size="sm" onClick={onRefresh}>
        <RefreshCwGlassIcon className="size-5" aria-hidden="true" />
        {t('timeRangeControls.refresh')}
      </Button>
      {children}
    </div>
  )
}
