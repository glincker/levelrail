import type { Tone } from '@/components/kit'
import type { UpstreamView } from './rollup'
import { SVG_TONE } from './svgTone'

interface GlyphProps {
  glyph: UpstreamView['glyph']
  cx: number
  cy: number
  tone: Tone
}

// Each state has its own outline (circle, triangle, square, dashed ring, slashed
// circle) so health never depends on color alone.
export function NodeGlyph({ glyph, cx, cy, tone }: GlyphProps) {
  const t = SVG_TONE[tone]
  const line = `${t.stroke} fill-none`
  switch (glyph) {
    case 'ok':
      return (
        <g aria-hidden="true">
          <circle cx={cx} cy={cy} r={9} className={line} strokeWidth={2} />
          <path
            d={`M ${cx - 4} ${cy} l 3 3 l 5 -6`}
            className={line}
            strokeWidth={2}
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </g>
      )
    case 'warn':
      return (
        <g aria-hidden="true">
          <path
            d={`M ${cx} ${cy - 9} L ${cx + 9} ${cy + 8} L ${cx - 9} ${cy + 8} Z`}
            className={line}
            strokeWidth={2}
            strokeLinejoin="round"
          />
          <path
            d={`M ${cx} ${cy - 3} v 5`}
            className={line}
            strokeWidth={2}
            strokeLinecap="round"
          />
        </g>
      )
    case 'down':
      return (
        <g aria-hidden="true">
          <rect
            x={cx - 9}
            y={cy - 9}
            width={18}
            height={18}
            rx={3}
            className={line}
            strokeWidth={2}
          />
          <path
            d={`M ${cx - 4} ${cy - 4} l 8 8 M ${cx + 4} ${cy - 4} l -8 8`}
            className={line}
            strokeWidth={2}
            strokeLinecap="round"
          />
        </g>
      )
    case 'off':
      return (
        <g aria-hidden="true">
          <circle cx={cx} cy={cy} r={9} className={line} strokeWidth={2} />
          <path
            d={`M ${cx - 6} ${cy + 6} L ${cx + 6} ${cy - 6}`}
            className={line}
            strokeWidth={2}
            strokeLinecap="round"
          />
        </g>
      )
    default:
      return (
        <circle
          cx={cx}
          cy={cy}
          r={9}
          className={line}
          strokeWidth={2}
          strokeDasharray="3 3"
          aria-hidden="true"
        />
      )
  }
}
