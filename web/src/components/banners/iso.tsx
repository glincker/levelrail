import './iso.css'
import { FLOOR, type Palette } from './isoTokens'

// 2:1 isometric block: (x, y) is the centre of the top face, a its half width.
export function Block({
  x,
  y,
  a,
  h,
  p,
}: {
  x: number
  y: number
  a: number
  h: number
  p: Palette
}) {
  const b = a / 2
  return (
    <g>
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
  const ring = Math.max(Math.abs(u), Math.abs(v))
  return {
    x: 290 + (u - v) * 46,
    y: 160 + (u + v) * 23,
    k: i,
    o: ring === 2 ? 'opacity-30' : ring === 1 ? 'opacity-60' : 'opacity-100',
  }
})

// Floor slab plus a dotted grid that fades towards its edges.
export function IsoFloor() {
  return (
    <>
      <Block x={290} y={160} a={200} h={8} p={FLOOR} />
      {DOTS.map((d) => (
        <circle
          key={d.k}
          cx={d.x}
          cy={d.y}
          r="1.8"
          className={`fill-(--iso-dot) ${d.o}`}
        />
      ))}
    </>
  )
}
