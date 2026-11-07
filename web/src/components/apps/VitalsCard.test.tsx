import { afterEach, describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/react'
import { VitalsCard } from './VitalsCard'
import { jsonResponse, renderWithProviders } from '../stacks/stacksTestUtils'

afterEach(() => {
  vi.unstubAllGlobals()
})

const stat = (p75: number, rating: string) => ({
  count: 5,
  p50: p75,
  p75,
  p95: p75,
  rating,
})

describe('VitalsCard', () => {
  it('shows p75 values with rating tones and the snippet', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          jsonResponse({
            enabled: true,
            public_key: 'abc123',
            days: 7,
            overall: { LCP: stat(2000, 'good'), CLS: stat(0.3, 'poor') },
            routes: [{ route: '/', metrics: { LCP: stat(2000, 'good') } }],
          }),
        ),
      ),
    )
    renderWithProviders(<VitalsCard appName="web" />)
    const lcp = await screen.findAllByText('2000 ms')
    expect(lcp[0]?.className).toContain('text-tone-success')
    expect(screen.getAllByText('0.3')[0]?.className).toContain(
      'text-tone-danger',
    )
    expect(screen.getByText(/data-key="abc123"/)).toBeTruthy()
  })

  it('shows the empty state when collection is off', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          jsonResponse({
            enabled: false,
            public_key: '',
            days: 7,
            overall: {},
            routes: [],
          }),
        ),
      ),
    )
    renderWithProviders(<VitalsCard appName="web" />)
    expect(await screen.findByText('emptyDisabled')).toBeTruthy()
  })

  it('renders a lower bound when a percentile overflows the top bucket', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          jsonResponse({
            enabled: true,
            public_key: 'abc123',
            days: 7,
            overall: { LCP: { ...stat(10000, 'poor'), overflow: true } },
            routes: [],
          }),
        ),
      ),
    )
    renderWithProviders(<VitalsCard appName="web" />)
    expect((await screen.findAllByText('>10 s'))[0]).toBeTruthy()
  })
})
