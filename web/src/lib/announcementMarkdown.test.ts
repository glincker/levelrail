import { describe, expect, it } from 'vitest'
import {
  announcementPlainText,
  isAllowedLink,
  parseAnnouncementText,
} from './announcementMarkdown'

describe('parseAnnouncementText', () => {
  it('splits paragraphs and reads bold, code and allowed links', () => {
    const p = parseAnnouncementText(
      'Update **now** with `lr upgrade`.\n\nSee [notes](https://levelrail.com/changelog).',
    )
    expect(p).toHaveLength(2)
    expect(p[0]).toContainEqual({ kind: 'bold', text: 'now' })
    expect(p[0]).toContainEqual({ kind: 'code', text: 'lr upgrade' })
    expect(p[1]).toContainEqual({
      kind: 'link',
      text: 'notes',
      href: 'https://levelrail.com/changelog',
    })
  })

  it('keeps HTML as literal text', () => {
    const p = parseAnnouncementText(
      '<script>alert(1)</script><img src=x onerror=y>',
    )
    expect(p[0] ?? []).toEqual([
      { kind: 'text', text: '<script>alert(1)</script><img src=x onerror=y>' },
    ])
  })

  it('does not link javascript: or data: urls', () => {
    for (const bad of [
      'javascript:alert(1)',
      'data:text/html,x',
      'JaVaScRiPt:x',
    ]) {
      const p = parseAnnouncementText(`[go](${bad})`)
      const row = p[0] ?? []
      expect(row.some((t) => t.kind === 'link')).toBe(false)
      expect(row.map((t) => t.text).join('')).toContain('[go](')
    }
  })

  it('rejects hosts outside the allowlist', () => {
    for (const bad of [
      'https://evil.com/levelrail.com',
      'https://levelrail.com.evil.com/',
      'https://github.com/other/repo',
      'https://github.com/glincker-evil/x',
      'http://levelrail.com/',
      'https://user:pw@levelrail.com/',
      'https://levelrail.com:8443/',
    ]) {
      expect(isAllowedLink(bad)).toBe(false)
    }
    expect(isAllowedLink('https://docs.levelrail.com/a')).toBe(true)
    expect(
      isAllowedLink('https://github.com/glincker/levelrail/releases'),
    ).toBe(true)
  })

  it('treats nested and unbalanced markup as plain text', () => {
    const p = parseAnnouncementText(
      '**bold `code`** and **open and `tick [x](https://levelrail.com',
    )
    expect((p[0] ?? []).filter((t) => t.kind === 'link')).toHaveLength(0)
    expect((p[0] ?? []).find((t) => t.kind === 'bold')).toEqual({
      kind: 'bold',
      text: 'bold `code`',
    })
    expect(announcementPlainText('**a** [b](javascript:x)')).toBe(
      'a [b](javascript:x)',
    )
  })
})
