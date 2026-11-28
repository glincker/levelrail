// @vitest-environment jsdom
import { renderToStaticMarkup } from 'react-dom/server'
import jsQR from 'jsqr'
import { describe, expect, it } from 'vitest'
import BrandQr from './BrandQr'
import {
  QR_ACCENT_PERCENT,
  QR_QUIET,
  buildQrModel,
  finderOrigins,
} from './qrModel'

const SCALE = 8

interface Box {
  x: number
  y: number
  w: number
  rx: number
}

function inRounded(mx: number, my: number, b: Box): boolean {
  if (mx < b.x || my < b.y || mx >= b.x + b.w || my >= b.y + b.w) return false
  const cx = Math.min(Math.max(mx, b.x + b.rx), b.x + b.w - b.rx)
  const cy = Math.min(Math.max(my, b.y + b.rx), b.y + b.w - b.rx)
  return (mx - cx) ** 2 + (my - cy) ** 2 <= b.rx ** 2
}

// jsdom has no canvas, so the bitmap is painted from the drawn SVG itself:
// finder rects and circles exactly, data modules from each sub-path start.
function rasterize(markup: string) {
  const doc = new DOMParser().parseFromString(markup, 'text/html')
  const svg = doc.querySelector('svg')
  if (!svg) throw new Error('no svg')
  const full = Number(svg.getAttribute('viewBox')?.split(' ')[2])
  const px = full * SCALE
  const data = new Uint8ClampedArray(px * px * 4).fill(255)
  const paint = (x: number, y: number, dark: boolean) => {
    const i = (y * px + x) * 4
    const v = dark ? 0 : 255
    data[i] = v
    data[i + 1] = v
    data[i + 2] = v
  }
  const cell = (cx: number, cy: number, dark: boolean) => {
    for (let y = 0; y < SCALE; y++)
      for (let x = 0; x < SCALE; x++)
        paint(cx * SCALE + x, cy * SCALE + y, dark)
  }
  const d = svg.querySelector('path[data-qr="data"]')?.getAttribute('d') ?? ''
  const starts = [...d.matchAll(/M(\d+) (\d+)\.5/g)].map((m) => [
    Number(m[1]),
    Number(m[2]),
  ])
  for (const [x = 0, y = 0] of starts) cell(x + QR_QUIET, y + QR_QUIET, true)
  for (const g of svg.querySelectorAll('g[data-qr="finder"]')) {
    const [outer, ring, eye] = [...g.querySelectorAll('rect')].map(
      (r): Box => ({
        x: Number(r.getAttribute('x')) + QR_QUIET,
        y: Number(r.getAttribute('y')) + QR_QUIET,
        w: Number(r.getAttribute('width')),
        rx: Number(r.getAttribute('rx')),
      }),
    )
    if (!outer || !ring || !eye) continue
    for (let y = 0; y < px; y++) {
      for (let x = 0; x < px; x++) {
        const mx = (x + 0.5) / SCALE
        const my = (y + 0.5) / SCALE
        if (inRounded(mx, my, outer)) {
          paint(x, y, !inRounded(mx, my, ring) || inRounded(mx, my, eye))
        }
      }
    }
  }
  return { data, px, starts }
}

const URLS = [
  'https://a.io',
  'http://shop.example.com:8080/menu?table=12&lang=en',
  `https://example.com/${'long-path-segment/'.repeat(8)}end`,
  'https://bücher.example.com/café/日本語',
]

describe('BrandQr', () => {
  it.each(URLS)('decodes %s from its drawn shapes', (url) => {
    const { data, px } = rasterize(
      renderToStaticMarkup(<BrandQr value={url} size={160} label="QR" />),
    )
    const res = jsQR(data, px, px)
    expect(res?.data).toBe(url)
  })

  it('draws exactly the encoder data modules plus three finders', () => {
    const url = 'https://a.io/x'
    const model = buildQrModel(url)
    const { starts } = rasterize(
      renderToStaticMarkup(<BrandQr value={url} size={160} label="QR" />),
    )
    const drawn = new Set(starts.map(([x, y]) => `${x},${y}`))
    const expected = new Set<string>()
    const finders = finderOrigins(model.size)
    model.dark.forEach((row, y) =>
      row.forEach((on, x) => {
        const inF = finders.some(
          ([fx, fy]) => x >= fx && x < fx + 7 && y >= fy && y < fy + 7,
        )
        if (on && !inF) expected.add(`${x},${y}`)
      }),
    )
    expect(drawn).toEqual(expected)
    const html = renderToStaticMarkup(
      <BrandQr value={url} size={160} label="QR" />,
    )
    expect(html.match(/data-qr="finder"/g)).toHaveLength(3)
  })

  it('keeps the quiet zone and a light tile in any theme', () => {
    const html = renderToStaticMarkup(
      <div className="dark">
        <BrandQr value="https://a.io" size={160} label="QR" />
      </div>,
    )
    const model = buildQrModel('https://a.io')
    expect(html).toContain(`viewBox="0 0 ${model.size + QR_QUIET * 2} `)
    expect(html).toContain('class="fill-white"')
  })

  it('darkens the default brand accent to at least 7:1 on white', () => {
    const lum = (rgb: number[]) => {
      const [r = 0, g = 0, b = 0] = rgb.map((v) => {
        const c = v / 255
        return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
      })
      return 0.2126 * r + 0.7152 * g + 0.0722 * b
    }
    const accent = [0x10, 0x72, 0x92]
    const mixed = accent.map((v) => (v * QR_ACCENT_PERCENT) / 100)
    expect(1.05 / (lum(mixed) + 0.05)).toBeGreaterThanOrEqual(7)
  })
})
