import { describe, expect, it } from 'vitest'
import type { LocalModelCard } from '../types/modelsCpu'
import {
  clamp,
  downloadPercent,
  formatCpus,
  formatGiB,
  formatMemoryMiB,
  sortCards,
  speedRange,
  suggestedName,
  vendorLogoId,
  verdictTone,
} from './modelsCpu'

describe('modelsCpu helpers', () => {
  it.each([
    [1023645440, '1.0 GB'],
    [2099501664, '2.0 GB'],
    [12 * 2 ** 30, '12 GB'],
  ])('formats %d bytes as %s', (bytes, want) => {
    expect(formatGiB(bytes)).toBe(want)
  })

  it('formats cpus and memory caps', () => {
    expect(formatCpus(2000)).toBe('2')
    expect(formatCpus(1500)).toBe('1.5')
    expect(formatMemoryMiB(4096)).toBe('4.0 GB')
  })

  it.each([
    ['fits_comfortably', 'success'],
    ['tight', 'warning'],
    ['wont_fit', 'danger'],
    ['unknown', 'neutral'],
  ] as const)('tones %s as %s', (verdict, tone) => {
    expect(verdictTone(verdict)).toBe(tone)
  })

  it('shows a speed range only when there is an estimate', () => {
    expect(speedRange(1.94, 5.3)).toBe('1.9 to 5.3')
    expect(speedRange(0, 0)).toBe('')
  })

  it('sorts light before default before plus, then by size', () => {
    const card = (id: string, tier: LocalModelCard['tier'], size: number) =>
      ({ id, tier, download_bytes: size }) as LocalModelCard
    const sorted = sortCards([
      card('p', 'plus', 5),
      card('d', 'default', 3),
      card('l2', 'light', 2),
      card('l1', 'light', 1),
    ])
    expect(sorted.map((c) => c.id)).toEqual(['l1', 'l2', 'd', 'p'])
  })

  it('maps known vendors to a mark and others to the monogram', () => {
    expect(vendorLogoId('ibm')).toBe('local-granite')
    expect(vendorLogoId('nomic')).toBeUndefined()
  })

  it('derives a valid resource name from a catalog id', () => {
    expect(suggestedName('granite-4.1-3b')).toBe('granite-4-1-3b')
  })

  it('clamps progress', () => {
    expect(downloadPercent(50, 200)).toBe(25)
    expect(downloadPercent(10, 0)).toBe(0)
    expect(downloadPercent(300, 200)).toBe(100)
    expect(clamp(5, 1, 3)).toBe(3)
  })
})
