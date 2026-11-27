import { describe, expect, it } from 'vitest'
import {
  formatNodePrices,
  parseNodePrices,
  totalMonthlySaving,
} from './advisorSavings'
import { classifyInstallBlock } from './templatePlatform'
import type { AdvisorFinding } from '../types/resourceAdvisor'

const finding = (monthly?: number): AdvisorFinding =>
  ({ impact: { monthly_saving: monthly } }) as AdvisorFinding

describe('node prices', () => {
  it('round trips the setting text', () => {
    const rows = parseNodePrices('local=12.5, eu-1=24')
    expect(rows).toEqual([
      { node: 'local', price: '12.5' },
      { node: 'eu-1', price: '24' },
    ])
    expect(formatNodePrices(rows)).toBe('local=12.5,eu-1=24')
  })

  it('rejects a bad row', () => {
    expect(formatNodePrices([{ node: 'a', price: '0' }])).toBeNull()
    expect(formatNodePrices([{ node: '', price: '5' }])).toBeNull()
  })
})

describe('totalMonthlySaving', () => {
  it('is null when nothing is priced, never zero', () => {
    expect(totalMonthlySaving([finding(), finding()])).toBeNull()
    expect(totalMonthlySaving([finding(3), finding()])).toBe(3)
  })
})

describe('classifyInstallBlock', () => {
  it('names the preflight refusals', () => {
    expect(
      classifyInstallBlock(422, 'repeat with allow_platform_mismatch.'),
    ).toEqual({ kind: 'arch' })
    expect(
      classifyInstallBlock(
        422,
        'template "x" needs the wireguard kernel module',
      ),
    ).toEqual({ kind: 'kernel', module: 'wireguard' })
    expect(
      classifyInstallBlock(403, 'capability net_admin needs the root ability'),
    ).toEqual({
      kind: 'root',
    })
    expect(classifyInstallBlock(500, 'boom')).toBeNull()
  })
})
