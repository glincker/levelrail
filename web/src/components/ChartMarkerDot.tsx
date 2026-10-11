import type { ChartMarker } from '../lib/metricChart'

// A marker dot drawn inside a recharts SVG. It is a focusable button when
// onClick is given; a native <title> carries the tooltip text.
export function ChartMarkerDot({
  marker,
  x,
  y,
  onClick,
}: {
  marker: ChartMarker
  x: number
  y: number
  onClick?: (marker: ChartMarker) => void
}) {
  const interactive = Boolean(onClick)
  return (
    <g
      role={interactive ? 'button' : undefined}
      tabIndex={interactive ? 0 : undefined}
      aria-label={interactive ? marker.tooltip : undefined}
      className={
        interactive
          ? 'cursor-pointer focus-visible:outline-2 focus-visible:outline-offset-2'
          : undefined
      }
      onClick={(e) => {
        if (onClick) {
          e.stopPropagation()
          onClick(marker)
        }
      }}
      onKeyDown={(e) => {
        if (onClick && (e.key === 'Enter' || e.key === ' ')) {
          e.preventDefault()
          e.stopPropagation()
          onClick(marker)
        }
      }}
    >
      <title>{marker.tooltip}</title>
      <circle
        cx={x}
        cy={y}
        r={interactive ? 6 : 4}
        fill={marker.color}
        stroke="currentColor"
        className="text-card"
        strokeWidth={1.5}
      />
    </g>
  )
}
