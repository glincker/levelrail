import { ArrowsClockwiseIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from './ui/button'
import { TIME_RANGE_PRESETS, type TimeRangeKey } from '../lib/timeRange'

// The preset-button group plus Refresh button shared by
// MetricsDashboard.tsx (per-app) and NodeMetricsDashboard.tsx
// (per-node): both dashboards let the viewer pick a `TimeRangeKey`
// preset and force a "now" recompute without touching that key, and
// until this extraction the two files carried character-for-character
// identical JSX for it. State (rangeKey, refreshNonce, the memoized
// resolveTimeRange call) stays in each dashboard, not here: this
// component is presentational only, driven entirely by props, the same
// split MetricChartCard.tsx draws between itself (presentational) and
// its two callers (data-fetching).

export function TimeRangeControls({
  rangeKey,
  onRangeChange,
  onRefresh,
}: {
  rangeKey: TimeRangeKey
  onRangeChange: (key: TimeRangeKey) => void
  onRefresh: () => void
}) {
  return (
    <div className="flex items-center gap-2">
      <div
        role="group"
        aria-label="Time range"
        className="inline-flex rounded-md border border-border"
      >
        {TIME_RANGE_PRESETS.map((preset) => (
          <button
            key={preset.key}
            type="button"
            onClick={() => {
              onRangeChange(preset.key)
            }}
            aria-pressed={rangeKey === preset.key}
            className={`px-2.5 py-1 text-xs font-medium first:rounded-l-md last:rounded-r-md ${
              rangeKey === preset.key
                ? 'bg-foreground text-background'
                : 'bg-transparent text-muted-foreground hover:bg-muted hover:text-foreground'
            }`}
          >
            {preset.label}
          </button>
        ))}
      </div>
      <Button type="button" variant="outline" size="sm" onClick={onRefresh}>
        <ArrowsClockwiseIcon className="size-3.5" aria-hidden="true" />
        Refresh
      </Button>
    </div>
  )
}
