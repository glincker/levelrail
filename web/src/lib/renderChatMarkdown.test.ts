import { describe, expect, it } from 'vitest'
import { renderChatMarkdown } from './renderChatMarkdown'

describe('renderChatMarkdown', () => {
  it('escapes quotes in link hrefs so attributes cannot be injected', () => {
    const html = renderChatMarkdown(
      '[x](https://a.test/"onmouseover="alert(1))',
    )
    expect(html).not.toMatch(/"onmouseover=/)
    expect(html).not.toMatch(/\sonmouseover=/)
  })

  it('renders javascript: links as plain text', () => {
    const html = renderChatMarkdown('[x](javascript:alert(1))')
    expect(html).not.toContain('<a')
  })

  it('keeps ordinary https links', () => {
    expect(
      renderChatMarkdown('[docs](https://example.com/a?b=1&c=2)'),
    ).toContain('href="https://example.com/a?b=1&amp;c=2"')
  })

  it('renders protocol-relative and backslash links as plain text', () => {
    expect(renderChatMarkdown('[x](//example.com/a)')).not.toContain('<a')
    expect(renderChatMarkdown('[x](/\\example.com)')).not.toContain('<a')
  })

  it('keeps same-origin path links', () => {
    expect(renderChatMarkdown('[x](/stacks/1)')).toContain('href="/stacks/1"')
  })
})
