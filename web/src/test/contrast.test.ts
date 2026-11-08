import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import path from 'node:path'

const read = (rel: string) =>
  readFileSync(path.resolve(import.meta.dirname, rel), 'utf8').replace(
    /\/\*[\s\S]*?\*\//g,
    '',
  )
// Order matters: later declarations win, like the cascade.
const css = read('../styles/glinr-tokens.css') + '\n' + read('../index.css')

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

type Color = { rgb: Rgb; alpha: number }

const RULE = /([^{}]+)\{([^{}]*)\}/g

// Collects raw declarations of rules whose selector list contains one of `targets`.
function decls(targets: string[]): Map<string, string> {
  const map = new Map<string, string>()
  for (const m of css.matchAll(RULE)) {
    const sels = (m[1] ?? '').split(',').map((x) => x.trim())
    if (!sels.some((x) => targets.includes(x))) continue
    for (const d of (m[2] ?? '').matchAll(/--([a-z0-9-]+):\s*([^;]+);/g)) {
      map.set(d[1] ?? '', (d[2] ?? '').trim())
    }
  }
  return map
}

function parseColor(
  raw: string,
  raws: Map<string, string>,
  depth = 0,
): Color | undefined {
  const v = raw.trim()
  const ref = /^var\(--([a-z0-9-]+)(?:,[^)]*)?\)$/.exec(v)
  if (ref) {
    const next = raws.get(ref[1] ?? '')
    return next && depth < 8 ? parseColor(next, raws, depth + 1) : undefined
  }
  const m =
    /^oklch\(([\d.]+)\s+([\d.]+)\s+([\d.]+)(?:\s*\/\s*([\d.]+)(%?))?\s*\)$/.exec(
      v,
    )
  if (!m) return undefined
  const alpha =
    m[4] === undefined ? 1 : m[5] ? Number(m[4]) / 100 : Number(m[4])
  return { rgb: oklchToRgb(Number(m[1]), Number(m[2]), Number(m[3])), alpha }
}

// Translucent colors are flattened onto the theme background, as they render.
function theme(selectors: string[][]): Map<string, Rgb> {
  const raws = new Map<string, string>()
  for (const group of selectors) {
    for (const [k, v] of decls(group)) raws.set(k, v)
  }
  const bg = parseColor(raws.get('background') ?? '', raws)?.rgb ?? [1, 1, 1]
  const out = new Map<string, Rgb>()
  for (const [k, v] of raws) {
    const c = parseColor(v, raws)
    if (!c) continue
    out.set(
      k,
      c.rgb.map((x, i) => x * c.alpha + (bg[i] ?? 0) * (1 - c.alpha)) as Rgb,
    )
  }
  return out
}

const LIGHT = [':root', '[data-mode="light"]']
const DARK = ['.dark', '[data-mode="dark"]']
const OLED = [':root.dark.oled']
const light = theme([LIGHT])
const dark = theme([LIGHT, DARK])
const oled = theme([LIGHT, DARK, OLED])

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

// Non-text UI parts (control borders, focus rings) need 3:1 (WCAG 1.4.11).
const uiPairs: [string, string, number][] = [
  ['glinr-border-control', 'background', 3],
  ['glinr-border-control', 'card', 3],
  ['ring', 'background', 3],
  ['primary', 'background', 3],
]

describe.each([
  ['light', light],
  ['dark', dark],
  ['oled', oled],
])('%s theme text contrast (WCAG AA 4.5:1)', (_name, theme) => {
  it.each(pairs)('%s on %s', (fg, bg, min) => {
    const f = theme.get(fg)
    const b = theme.get(bg)
    expect(f, fg).toBeDefined()
    expect(b, bg).toBeDefined()
    expect(ratio(f as Rgb, b as Rgb)).toBeGreaterThanOrEqual(min)
  })
})

describe.each([
  ['light', light],
  ['dark', dark],
  ['oled', oled],
])('%s theme UI contrast (WCAG 1.4.11 3:1)', (_name, theme) => {
  it.each(uiPairs)('%s on %s', (fg, bg, min) => {
    const f = theme.get(fg)
    const b = theme.get(bg)
    expect(f, fg).toBeDefined()
    expect(b, bg).toBeDefined()
    expect(ratio(f as Rgb, b as Rgb)).toBeGreaterThanOrEqual(min)
  })
})
