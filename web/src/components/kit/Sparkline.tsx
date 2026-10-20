import { useEffect, useRef, useState } from 'react'
import { cn } from '@/lib/utils'
import { TONE, type Tone } from './tone'
import { areaPath, smoothPath, sparkPoints } from './sparkPath'

export interface SparklineProps {
  values: number[]
  tone?: Tone
  height?: number
  width?: number | 'fill'
  fill?: boolean
  ariaLabel: string
  showLast?: boolean
}

const FALLBACK_WIDTH = 120

export function Sparkline({
  values,
  tone = 'neutral',
  height = 32,
  width = 96,
  fill = false,
  ariaLabel,
  showLast = false,
}: SparklineProps) {
  const ref = useRef<HTMLSpanElement>(null)
  const [measured, setMeasured] = useState(FALLBACK_WIDTH)

  useEffect(() => {
    if (width !== 'fill') return
    const el = ref.current
    if (!el) return
    const measure = () => {
      const w = el.getBoundingClientRect().width
      if (w > 0) setMeasured(w)
    }
    measure()
    if (typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [width])

  const w = width === 'fill' ? measured : width
  const pts = sparkPoints(values, w, height)
  const line = smoothPath(pts, height)
  const last = pts[pts.length - 1]

  return (
    <span
      ref={ref}
      className={cn(
        'inline-block align-middle',
        TONE[tone].text,
        width === 'fill' && 'block w-full',
      )}
      style={width === 'fill' ? { height } : undefined}
    >
      <svg
        role="img"
        aria-label={ariaLabel}
        width={w}
        height={height}
        viewBox={`0 0 ${w} ${height}`}
        className="block overflow-visible"
      >
        {pts.length === 0 && (
          <line
            data-testid="spark-empty"
            x1={2}
            x2={Math.max(w - 2, 2)}
            y1={height / 2}
            y2={height / 2}
            stroke="currentColor"
            strokeOpacity={0.3}
            strokeDasharray="2 3"
            strokeLinecap="round"
          />
        )}
        {fill && pts.length > 1 && (
          <path
            data-testid="spark-area"
            d={areaPath(line, pts, height)}
            fill="currentColor"
            fillOpacity={0.12}
          />
        )}
        {pts.length > 1 && (
          <path
            data-testid="spark-line"
            d={line}
            fill="none"
            stroke="currentColor"
            strokeWidth={1.75}
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        )}
        {last && (showLast || pts.length === 1) && (
          <circle
            data-testid="spark-last"
            cx={last.x}
            cy={last.y}
            r={2.5}
            fill="currentColor"
          />
        )}
      </svg>
    </span>
  )
}
