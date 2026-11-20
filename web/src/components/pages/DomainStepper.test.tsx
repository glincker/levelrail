import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { DomainStepper } from './DomainStepper'
import { domainSteps, redirectLine } from './pageHelpers'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))

describe('DomainStepper', () => {
  it('is live only when DNS and certificate are both done', () => {
    expect(domainSteps(true, false)).toEqual({
      dns: 'done',
      cert: 'pending',
      live: 'pending',
    })
    expect(domainSteps(true, true).live).toBe('done')
  })

  it('builds a copyable redirect line', () => {
    expect(redirectLine(' /old ', '/new', '301')).toBe('/old /new 301')
  })

  it('names each step and its state for assistive tech', () => {
    render(<DomainStepper dnsOk certOk={false} />)
    const items = screen.getAllByRole('listitem')
    expect(items).toHaveLength(3)
    expect(items[0]?.textContent).toContain('domains.done')
    expect(items[1]?.textContent).toContain('domains.waiting')
  })
})
