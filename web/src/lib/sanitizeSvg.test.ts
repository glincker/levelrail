import { describe, expect, it } from 'vitest'
import { sanitizeSvg } from './sanitizeSvg'

describe('sanitizeSvg', () => {
  it('keeps a plain mark intact', () => {
    const out = sanitizeSvg(
      '<svg viewBox="0 0 10 10" xmlns="http://www.w3.org/2000/svg"><path d="M0 0h10v10z" fill="#fff"/></svg>',
    )
    expect(out).toContain('<path')
    expect(out).toContain('viewBox="0 0 10 10"')
    expect(out).toContain('fill="#fff"')
  })

  it('strips scripts, foreignObject and event handlers', () => {
    const out = sanitizeSvg(
      '<svg onload="alert(1)"><script>alert(1)</script><foreignObject><div>x</div></foreignObject><circle r="4" onclick="alert(2)" onmouseover="x()"/></svg>',
    )
    expect(out.toLowerCase()).not.toContain('script')
    expect(out.toLowerCase()).not.toContain('foreignobject')
    expect(out.toLowerCase()).not.toMatch(/\son[a-z]+=/)
    expect(out).toContain('<circle')
  })

  it('drops javascript: and data: hrefs, keeps fragment refs', () => {
    const out = sanitizeSvg(
      '<svg xmlns:xlink="http://www.w3.org/1999/xlink"><a href="javascript:alert(1)"><circle r="1"/></a><a xlink:href=" JaVaScRiPt:alert(1)"><circle r="2"/></a><use href="#g"/><use href="data:image/svg+xml;base64,AAAA"/></svg>',
    )
    expect(out.toLowerCase()).not.toContain('javascript')
    expect(out).not.toContain('data:')
    expect(out).toContain('href="#g"')
  })

  it('keeps benign inline styles and drops hostile ones', () => {
    const out = sanitizeSvg(
      '<svg><rect style="fill:#000" width="1" height="1"/><rect style="fill:url(https://evil/x)" width="1" height="1"/></svg>',
    )
    expect(out).toContain('fill:#000')
    expect(out).not.toContain('evil')
  })

  it('removes style blocks and unknown elements, and returns empty for non-svg input', () => {
    expect(
      sanitizeSvg(
        '<svg><style>@import url(//evil)</style><iframe src="x"/><rect width="1" height="1"/></svg>',
      ).toLowerCase(),
    ).not.toMatch(/style|iframe/)
    expect(sanitizeSvg('<img src=x onerror=alert(1)>')).toBe('')
    expect(sanitizeSvg('')).toBe('')
  })

  it('drops external use hrefs but keeps fragment refs', () => {
    const out = sanitizeSvg(
      '<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"><use href="https://x.test/a.svg#i"/><use xlink:href="https://x.test/b.svg#i"/><use href="#local"/></svg>',
    )
    expect(out).not.toContain('x.test')
    expect(out).toContain('href="#local"')
  })
})
