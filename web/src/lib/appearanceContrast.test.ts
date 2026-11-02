import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const css = readFileSync(resolve(__dirname, '../index.css'), 'utf8')

function block(
  selector: string,
  mustHave = '--background',
): Record<string, string> {
  const re = new RegExp(
    `${selector.replace(/[.[\]()*:]/g, '\\$&')}\\s*\\{([^}]*)\\}`,
    'g',
  )
  for (const m of css.matchAll(re)) {
    const body = m[1] ?? ''
    if (!body.includes(mustHave)) continue
    const out: Record<string, string> = {}
    for (const d of body.matchAll(/(--[\w-]+):\s*([^;]+);/g)) {
      out[d[1] ?? ''] = (d[2] ?? '').trim()
    }
    return out
  }
  throw new Error(`block not found: ${selector}`)
}

function oklchToLinear(value: string): [number, number, number] {
  const m = /oklch\(\s*([\d.]+)\s+([\d.]+)\s+([\d.]+)/.exec(value)
  if (!m) throw new Error(`not an opaque oklch color: ${value}`)
  const L = Number(m[1])
  const C = Number(m[2])
  const h = (Number(m[3]) * Math.PI) / 180
  const a = C * Math.cos(h)
  const b = C * Math.sin(h)
  const l = (L + 0.3963377774 * a + 0.2158037573 * b) ** 3
  const mm = (L - 0.1055613458 * a - 0.0638541728 * b) ** 3
  const s = (L - 0.0894841775 * a - 1.291485548 * b) ** 3
  const clamp = (x: number) => Math.min(1, Math.max(0, x))
  return [
    clamp(4.0767416621 * l - 3.3077115913 * mm + 0.2309699292 * s),
    clamp(-1.2684380046 * l + 2.6097574011 * mm - 0.3413193965 * s),
    clamp(-0.0041960863 * l - 0.7034186147 * mm + 1.707614701 * s),
  ]
}

function luminance(value: string): number {
  const [r, g, b] = oklchToLinear(value).map((c) =>
    c <= 0.0031308 ? 12.92 * c : 1.055 * c ** (1 / 2.4) - 0.055,
  ) as [number, number, number]
  const lin = (c: number) =>
    c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  return 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b)
}

function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x) as [
    number,
    number,
  ]
  return (hi + 0.05) / (lo + 0.05)
}

interface Base {
  name: string
  tokens: Record<string, string>
}

const darkDefault = block('.dark')
const lightDefault = block(':root')

function withDefaults(
  name: string,
  base: Record<string, string>,
  defaults: Record<string, string>,
): Base {
  return { name, tokens: { ...defaults, ...base } }
}

const bases: Base[] = [
  withDefaults('dark soft', block(':root.dark'), darkDefault),
  ...['black', 'graphite', 'midnight'].map((n) =>
    withDefaults(
      `dark ${n}`,
      block(`:root.dark[data-dark-base='${n}']`),
      darkDefault,
    ),
  ),
  withDefaults('light white', {}, lightDefault),
  ...['grey', 'paper', 'cool'].map((n) =>
    withDefaults(
      `light ${n}`,
      block(`:root:not(.dark)[data-light-base='${n}']`),
      lightDefault,
    ),
  ),
]

describe.each(bases)('appearance base $name', ({ tokens }) => {
  const t = (k: string) => tokens[k] ?? ''

  it('keeps body text at AA on background and card', () => {
    expect(
      contrast(t('--foreground'), t('--background')),
    ).toBeGreaterThanOrEqual(4.5)
    expect(
      contrast(t('--card-foreground'), t('--card')),
    ).toBeGreaterThanOrEqual(4.5)
  })

  it('keeps muted text at AA on background and card', () => {
    expect(
      contrast(t('--muted-foreground'), t('--background')),
    ).toBeGreaterThanOrEqual(4.5)
    expect(
      contrast(t('--muted-foreground'), t('--card')),
    ).toBeGreaterThanOrEqual(4.5)
  })
})

describe('non-default bases keep surfaces distinguishable', () => {
  it.each(
    bases.filter(
      (b) =>
        b.tokens['--border']?.startsWith('oklch(0.') &&
        !b.name.endsWith('soft') &&
        !b.name.endsWith('white'),
    ),
  )('$name border and card differ from background', ({ name, tokens }) => {
    const t = (k: string) => tokens[k] ?? ''
    // Light dividers are decoration; the card fill carries the separation there.
    const min = name.startsWith('dark') ? 1.3 : 1.2
    expect(contrast(t('--border'), t('--background'))).toBeGreaterThanOrEqual(
      min,
    )
    expect(contrast(t('--card'), t('--background'))).toBeGreaterThanOrEqual(
      name.startsWith('dark') ? 1.05 : 1.04,
    )
  })
})

describe('accent text stays AA on every surface', () => {
  const accents = [
    ...css.matchAll(
      /:root(\.dark)?\[data-accent='(\w+)'\]\s*\{\s*--state-accent:\s*(oklch\([^)]+\))/g,
    ),
  ].map((m): [string, string, string, string] => [
    m[0],
    m[1] ?? '',
    m[2] ?? '',
    m[3] ?? '',
  ])
  it('finds the accent overrides', () => {
    expect(accents.length).toBe(10)
  })
  it.each(accents)('%s', (_m, dark, name, value) => {
    const wanted = bases.filter((b) =>
      b.name.startsWith(dark ? 'dark' : 'light'),
    )
    for (const b of wanted) {
      expect(
        contrast(value ?? '', b.tokens['--card'] ?? ''),
        `${name} on ${b.name}`,
      ).toBeGreaterThanOrEqual(4.5)
    }
  })
})
