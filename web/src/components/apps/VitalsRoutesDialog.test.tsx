import { describe, expect, it } from 'vitest'
import { fireEvent, screen, within } from '@testing-library/react'
import VitalsRoutesDialog from './VitalsRoutesDialog'
import { renderWithProviders } from '../stacks/stacksTestUtils'
import { rateVital } from '../../lib/vitalsThresholds'
import type { AppVitals } from '../../queries/appVitals'

const stat = (p75: number, count = 5) => ({
  count,
  p50: p75,
  p75,
  p95: p75,
  rating: '' as const,
})

const data: AppVitals = {
  enabled: true,
  public_key: 'abc123',
  days: 7,
  overall: {},
  routes: [
    { route: '/a', metrics: { LCP: stat(3000, 9) } },
    { route: '/b', metrics: { LCP: stat(1000, 3) } },
    { route: '/c', metrics: { LCP: stat(5000, 6) } },
  ],
}

function routeOrder(): string[] {
  const rows = screen.getAllByRole('row').slice(1)
  return rows.map((r) => within(r).getAllByRole('cell')[0]?.textContent ?? '')
}

describe('VitalsRoutesDialog', () => {
  it('rates by the fixed thresholds', () => {
    expect(rateVital('LCP', 2500)).toBe('good')
    expect(rateVital('LCP', 3000)).toBe('needs-improvement')
    expect(rateVital('CLS', 0.3)).toBe('poor')
  })

  it('colours values and sorts by a metric column', () => {
    renderWithProviders(
      <VitalsRoutesDialog
        appName="web"
        data={data}
        open
        onOpenChange={() => {}}
      />,
    )
    expect(screen.getByText('1000 ms').className).toContain('text-tone-success')
    expect(screen.getByText('5000 ms').className).toContain('text-tone-danger')
    expect(routeOrder()).toEqual(['/a', '/c', '/b'])
    fireEvent.click(screen.getByRole('button', { name: 'LCP' }))
    expect(routeOrder()).toEqual(['/c', '/a', '/b'])
    fireEvent.click(screen.getByRole('button', { name: 'LCP' }))
    expect(routeOrder()).toEqual(['/b', '/a', '/c'])
  })

  it('shows the snippet in the empty state', () => {
    renderWithProviders(
      <VitalsRoutesDialog
        appName="web"
        data={{ ...data, routes: [] }}
        open
        onOpenChange={() => {}}
      />,
    )
    expect(screen.getByText('emptyEnabled')).toBeTruthy()
    expect(screen.getByText(/data-key="abc123"/)).toBeTruthy()
  })
})
