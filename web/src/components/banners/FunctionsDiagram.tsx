import { useTranslation } from 'react-i18next'
import './iso.css'

type Palette = { top: string; l: string; r: string }

const BLUE: Palette = {
  top: 'fill-(--iso-top)',
  l: 'fill-(--iso-l)',
  r: 'fill-(--iso-r)',
}
const GREY: Palette = {
  top: 'fill-(--iso-sleep-top)',
  l: 'fill-(--iso-sleep-l)',
  r: 'fill-(--iso-sleep-r)',
}
const FLOOR: Palette = {
  top: 'fill-(--iso-floor-top)',
  l: 'fill-(--iso-floor-l)',
  r: 'fill-(--iso-floor-r)',
}
const YELLOW: Palette = {
  top: 'fill-(--iso-accent-top)',
  l: 'fill-(--iso-accent-l)',
  r: 'fill-(--iso-accent-r)',
}

// 2:1 isometric block: (x, y) is the centre of the top face, a its half width.
function Block({
  x,
  y,
  a,
  h,
  p,
  className,
}: {
  x: number
  y: number
  a: number
  h: number
  p: Palette
  className?: string
}) {
  const b = a / 2
  return (
    <g className={className}>
      <path
        d={`M${x - a} ${y}L${x} ${y + b}V${y + b + h}L${x - a} ${y + h}Z`}
        className={p.l}
      />
      <path
        d={`M${x + a} ${y}L${x} ${y + b}V${y + b + h}L${x + a} ${y + h}Z`}
        className={p.r}
      />
      <path
        d={`M${x} ${y - b}L${x + a} ${y}L${x} ${y + b}L${x - a} ${y}Z`}
        className={p.top}
      />
    </g>
  )
}

const DOTS = Array.from({ length: 25 }, (_, i) => {
  const u = (i % 5) - 2
  const v = Math.floor(i / 5) - 2
  return { x: 300 + (u - v) * 40, y: 140 + (u + v) * 20, k: i }
})

const LABEL =
  'fill-muted-foreground font-mono text-[8px] uppercase tracking-[0.18em]'

export function FunctionsDiagram() {
  const { t } = useTranslation('functions')
  return (
    <svg
      viewBox="0 0 480 260"
      className="h-full w-full max-w-full"
      fill="none"
      role="presentation"
    >
      <Block x={300} y={140} a={170} h={8} p={FLOOR} />
      {DOTS.map((d) => (
        <circle
          key={d.k}
          cx={d.x}
          cy={d.y}
          r="1.8"
          className="fill-(--iso-dot)"
        />
      ))}
      <path
        d="M92 125L184 79L266 120"
        className="stroke-(--iso-accent-l)"
        strokeWidth="1.5"
        strokeDasharray="4 4"
      />
      <path
        d="M354 120L430 82"
        className="stroke-(--iso-l)"
        strokeWidth="1.5"
        strokeDasharray="4 4"
      />
      <Block x={64} y={125} a={30} h={14} p={GREY} />
      <Block x={310} y={120} a={44} h={30} p={GREY} />
      <Block
        x={310}
        y={120}
        a={44}
        h={30}
        p={BLUE}
        className="opacity-100 motion-safe:animate-[iso-wake_6s_linear_infinite]"
      />
      <g transform="translate(184 79)">
        <g className="motion-safe:animate-[iso-travel_6s_linear_infinite]">
          <Block x={0} y={-8} a={11} h={11} p={YELLOW} />
        </g>
      </g>
      <g className="stroke-border" strokeWidth="1">
        <path d="M64 106V88" />
        <path d="M310 94V58" />
        <path d="M310 176V240" />
      </g>
      <g className={LABEL}>
        <text x="64" y="82" textAnchor="middle">
          {t('banner.diagram.request')}
        </text>
        <text x="310" y="52" textAnchor="middle">
          {t('banner.diagram.function')}
        </text>
        <text x="310" y="252" textAnchor="middle">
          {t('banner.diagram.sleeps')}
        </text>
      </g>
    </svg>
  )
}
