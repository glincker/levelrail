import { describe, expect, it } from 'vitest'
import {
  baseName,
  copySnippet,
  directUploadLimit,
  formatAssetSize,
  formatUsd,
  isImageAsset,
  sizeLabel,
} from './assetsFormat'

describe('assetsFormat', () => {
  it('formats sizes', () => {
    expect(formatAssetSize(0)).toBe('0 B')
    expect(formatAssetSize(1536)).toBe('1.5 KiB')
    expect(formatAssetSize(5 * 1024 * 1024)).toBe('5.0 MiB')
  })

  it('formats tiny prices with more digits', () => {
    expect(formatUsd(0)).toBe('$0.00')
    expect(formatUsd(0.0023)).toBe('$0.0023')
    expect(formatUsd(12.5)).toBe('$12.50')
  })

  it('detects images by type or extension', () => {
    expect(isImageAsset('a/b.PNG')).toBe(true)
    expect(isImageAsset('a/b.bin', 'image/webp')).toBe(true)
    expect(isImageAsset('a/b.zip')).toBe(false)
  })

  it('builds copy snippets', () => {
    expect(copySnippet('url', 'https://x/a.png', 'a.png')).toBe(
      'https://x/a.png',
    )
    expect(copySnippet('markdown', 'https://x/a.png', 'a.png')).toBe(
      '![a.png](https://x/a.png)',
    )
    expect(copySnippet('img', 'https://x/a.png', 'a.png')).toBe(
      '<img src="https://x/a.png" alt="a.png" />',
    )
  })

  it('takes the last path segment', () => {
    expect(baseName('a/b/c.png')).toBe('c.png')
    expect(baseName('c.png')).toBe('c.png')
  })

  it('renders at least when the listing was capped', () => {
    const f = (s: string) => `at least ${s}`
    expect(sizeLabel(2048, true, f)).toBe('at least 2.0 KiB')
    expect(sizeLabel(2048, false, f)).toBe('2.0 KiB')
  })

  it('has a positive upload threshold', () => {
    expect(directUploadLimit()).toBeGreaterThan(0)
  })
})
