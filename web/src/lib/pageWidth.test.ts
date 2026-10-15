import { describe, expect, it } from 'vitest'
import { getPageWidth, setPageWidth } from './pageWidth'

describe('pageWidth', () => {
  it('defaults to comfortable and stamps the root attribute', () => {
    expect(getPageWidth()).toBe('comfortable')
    expect(document.documentElement.getAttribute('data-width')).toBe(
      'comfortable',
    )
  })

  it('persists and applies a new width', () => {
    setPageWidth('full')
    expect(getPageWidth()).toBe('full')
    expect(document.documentElement.getAttribute('data-width')).toBe('full')
    expect(localStorage.getItem('ui-page-width')).toBe('full')
    setPageWidth('comfortable')
  })
})
