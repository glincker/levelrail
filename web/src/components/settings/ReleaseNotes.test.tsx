import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { stripNoteComments } from '../../lib/releaseNotes'
import { ReleaseNotes } from './ReleaseNotes'

describe('stripNoteComments', () => {
  it('removes release-note markers and unterminated comments', () => {
    expect(
      stripNoteComments('<!-- release-notes:start -->\nhello\n<!-- end -->'),
    ).toBe('hello')
    expect(stripNoteComments('keep <!-- never closed')).toBe('keep')
  })
})

describe('ReleaseNotes', () => {
  it('renders a GitHub alert and hides the marker text', () => {
    render(
      <ReleaseNotes
        markdown={'<!-- release-notes:start -->\n> [!NOTE]\n> Be careful\n'}
      />,
    )
    expect(screen.getByText(/Be careful/)).toBeTruthy()
    expect(screen.queryByText(/\[!NOTE\]/)).toBeNull()
    expect(screen.queryByText(/release-notes/)).toBeNull()
  })

  it('never injects raw HTML or unsafe links', () => {
    const { container } = render(
      <ReleaseNotes
        markdown={
          'Hi <img src=x onerror=alert(1)> <script>alert(1)</script>\n\n[bad](javascript:alert(1)) [ok](https://example.com)'
        }
      />,
    )
    expect(container.querySelector('script')).toBeNull()
    expect(container.querySelector('img')).toBeNull()
    const links = Array.from(container.querySelectorAll('a')).map((a) =>
      a.getAttribute('href'),
    )
    expect(links).toEqual(['https://example.com'])
  })
})
