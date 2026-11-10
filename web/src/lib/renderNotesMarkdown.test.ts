import { describe, expect, it } from 'vitest'
import { renderNotesMarkdown } from './renderNotesMarkdown'

describe('renderNotesMarkdown', () => {
  it.each([
    ['script tag', '<script>alert(1)</script> hi'],
    ['img onerror', '<img src=x onerror="alert(1)">'],
    ['inline onerror in html block', '<div onerror="alert(1)">x</div>'],
    ['svg onload', '<svg onload=alert(1)></svg>'],
  ])('drops raw html: %s', (_, src) => {
    const html = renderNotesMarkdown(src)
    expect(html).not.toMatch(/<script|onerror|onload|<img|<svg|<div/i)
  })

  it.each([
    ['javascript link', '[x](javascript:alert(1))'],
    ['mixed case javascript', '[x](JaVaScRiPt:alert(1))'],
    ['data link', '[x](data:text/html,<script>alert(1)</script>)'],
    ['relative link', '[x](/settings)'],
    ['protocol relative', '[x](//evil.example)'],
  ])('renders no anchor for unsafe href: %s', (_, src) => {
    const html = renderNotesMarkdown(src)
    expect(html).not.toContain('<a ')
    expect(html).not.toMatch(/javascript:/i)
  })

  it('keeps http and mailto links with noopener noreferrer', () => {
    const html = renderNotesMarkdown(
      '[runbook](https://runbooks.example/web) and [mail](mailto:ops@example.com)',
    )
    expect(html).toContain('href="https://runbooks.example/web"')
    expect(html).toContain('href="mailto:ops@example.com"')
    expect(html).toContain('rel="noopener noreferrer"')
  })

  it.each([
    ['entity encoded tab in scheme', '[x](jav&#x09;ascript:alert(1))'],
    ['hex entity scheme', '[x](&#x6A;avascript:alert(1))'],
    ['autolink', '<javascript:alert(1)>'],
    ['reference definition', '[x][r]\n\n[r]: javascript:alert(1)'],
    ['tab inside scheme', '[x](java\tscript:alert(1))'],
    ['space before scheme', '[x]( javascript:alert(1))'],
    ['vbscript', '[x](vbscript:msgbox(1))'],
  ])('no executable link: %s', (_, src) => {
    const html = renderNotesMarkdown(src)
    expect(html).not.toMatch(/href="\s*(javascript|vbscript|data):/i)
    expect(html).not.toMatch(/<a [^>]*href="[^"]*script:/i)
  })

  it('keeps code spans and fences as escaped text', () => {
    const html = renderNotesMarkdown(
      '`<script>alert(1)</script>`\n\n```\n<img src=x onerror=alert(1)>\n```',
    )
    expect(html).not.toMatch(/<script|<img/i)
    expect(html).toContain('&lt;script&gt;')
  })

  it('escapes link titles', () => {
    const html = renderNotesMarkdown(
      '[x](https://e.example "a" onmouseover="alert(1))',
    )
    expect(html).not.toMatch(/"\s*onmouseover=/)
  })

  it('drops images, even remote ones', () => {
    expect(
      renderNotesMarkdown('![x](https://tracker.example/p.png)'),
    ).not.toContain('<img')
  })

  it('escapes entities and quotes in text and hrefs', () => {
    expect(renderNotesMarkdown('a &lt;b&gt; < c')).not.toContain('<b>')
    const html = renderNotesMarkdown(
      '[x](https://e.example/"onmouseover="alert(1))',
    )
    expect(html).not.toMatch(/"\s*onmouseover=/)
  })
})
