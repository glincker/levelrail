const ALLOWED_ELEMENTS = new Set([
  'svg',
  'g',
  'defs',
  'path',
  'circle',
  'ellipse',
  'line',
  'polyline',
  'polygon',
  'rect',
  'text',
  'tspan',
  'title',
  'desc',
  'use',
  'symbol',
  'clippath',
  'mask',
  'lineargradient',
  'radialgradient',
  'stop',
  'pattern',
  'a',
])

const URL_ATTRS = new Set(['href', 'xlink:href'])
const SAFE_URL = /^(#|https?:|\/(?!\/))/i

function cleanElement(el: Element): void {
  for (const child of Array.from(el.children)) {
    if (!ALLOWED_ELEMENTS.has(child.localName.toLowerCase())) {
      child.remove()
      continue
    }
    cleanElement(child)
  }
  for (const attr of Array.from(el.attributes)) {
    const name = attr.name.toLowerCase()
    const value = attr.value.replace(/\s+/g, '')
    const dangerousUrl = URL_ATTRS.has(name) && !SAFE_URL.test(value)
    const urlRef = /url\(\s*['"]?\s*(?!#)/i.test(value)
    if (
      name.startsWith('on') ||
      dangerousUrl ||
      urlRef ||
      (name === 'style' && /expression|javascript:|@import/i.test(value))
    ) {
      el.removeAttribute(attr.name)
    }
  }
}

// Brand SVG is operator config, but a brand file can be shared between
// instances, so only a drawing allowlist is rendered.
export function sanitizeSvg(raw: string): string {
  if (!raw.trim()) return ''
  const doc = new DOMParser().parseFromString(raw, 'image/svg+xml')
  const root = doc.documentElement
  if (
    !root ||
    root.localName.toLowerCase() !== 'svg' ||
    doc.getElementsByTagName('parsererror').length > 0
  ) {
    return ''
  }
  cleanElement(root)
  return new XMLSerializer().serializeToString(root)
}
