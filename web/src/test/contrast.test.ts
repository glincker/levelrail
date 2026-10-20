import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import path from 'node:path'

const css = readFileSync(
  path.resolve(import.meta.dirname, '../index.css'),
  'utf8',
)

type Rgb = [number, number, number]

function oklchToRgb(l: number, c: number, hDeg: number): Rgb {
  const h = (hDeg * Math.PI) / 180
  const a = c * Math.cos(h)
  const b = c * Math.sin(h)
  const l_ = (l + 0.3963377774 * a + 0.2158037573 * b) ** 3
  const m_ = (l - 0.1055613458 * a - 0.0638541728 * b) ** 3
  const s_ = (l - 0.0894841775 * a - 1.291485548 * b) ** 3
  const lin: Rgb = [
    4.0767416621 * l_ - 3.3077115913 * m_ + 0.2309699292 * s_,
    -1.2684380046 * l_ + 2.6097574011 * m_ - 0.3413193965 * s_,
    -0.0041960863 * l_ - 0.7034186147 * m_ + 1.707614701 * s_,
  ]
  return lin.map((v) => Math.min(1, Math.max(0, v))) as Rgb
}

function luminance([r, g, b]: Rgb): number {
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

function ratio(fg: Rgb, bg: Rgb): number {
  const a = luminance(fg)
  const b = luminance(bg)
  const hi = Math.max(a, b)
  const lo = Math.min(a, b)
  return (hi + 0.05) / (lo + 0.05)
}

function block(selector: string): string {
  const out: string[] = []
  const re = new RegExp(`(?:^|\\n)${selector}\\s*\\{([^}]*)\\}`, 'g')
  for (const m of css.matchAll(re)) out.push(m[1] ?? '')
  return out.join('\n')
}

function tokens(body: string): Map<string, Rgb> {
  const map = new Map<string, Rgb>()
  for (const m of body.matchAll(
    /--([a-z0-9-]+):\s*oklch\(([\d.]+)\s+([\d.]+)\s+([\d.]+)\s*\)/g,
  )) {
    map.set(m[1] ?? '', oklchToRgb(Number(m[2]), Number(m[3]), Number(m[4])))
  }
  for (const m of body.matchAll(
    /--([a-z0-9-]+):\s*oklch\(([\d.]+)\s+0\s+0\)/g,
  )) {
    map.set(m[1] ?? '', oklchToRgb(Number(m[2]), 0, 0))
  }
  return map
}

const light = tokens(block(':root'))
const dark = new Map([...light, ...tokens(block('\\.dark'))])

const TONES = ['neutral', 'success', 'warning', 'danger', 'info', 'accent']
const pairs: [string, string, number][] = [
  ['foreground', 'background', 4.5],
  ['muted-foreground', 'background', 4.5],
  ['muted-foreground', 'muted', 4.5],
  ['primary-foreground', 'primary', 4.5],
  ['secondary-foreground', 'secondary', 4.5],
  ['accent-foreground', 'accent', 4.5],
  ['card-foreground', 'card', 4.5],
  ['destructive', 'background', 4.5],
  ...TONES.flatMap((t): [string, string, number][] => [
    [`tone-${t}-fg`, `tone-${t}-soft`, 4.5],
    [`tone-${t}-fg`, 'background', 4.5],
  ]),
]

describe.each([
  ['light', light],
  ['dark', dark],
])('%s theme text contrast (WCAG AA 4.5:1)', (_name, theme) => {
  it.each(pairs)('%s on %s', (fg, bg, min) => {
    const f = theme.get(fg)
    const b = theme.get(bg)
    expect(f, fg).toBeDefined()
    expect(b, bg).toBeDefined()
    expect(ratio(f as Rgb, b as Rgb)).toBeGreaterThanOrEqual(min)
  })
})
