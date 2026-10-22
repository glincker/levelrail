import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { NetworkSharesPage } from './NetworkSharesPage'
import { Providers } from './testProviders'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('NetworkSharesPage', () => {
  it('shows the hero banner with exactly one primary action and no empty-state card', () => {
    render(
      <Providers>
        <NetworkSharesPage shares={[]} />
      </Providers>,
    )
    expect(screen.getByTestId('page-banner-full')).toBeTruthy()
    expect(
      screen.getAllByRole('button', { name: /add network share/i }),
    ).toHaveLength(1)
    expect(screen.queryByText('No network shares')).toBeNull()
    expect(
      screen.getByText(/A share lets an app read and write files/),
    ).toBeTruthy()
  })

  it('keeps the table below a compact banner when shares exist', () => {
    render(
      <Providers>
        <NetworkSharesPage
          shares={[
            {
              id: 'ns_1',
              name: 'media-nas',
              protocol: 'nfs',
              host: 'nas.lan',
              remote_path: '/export/media',
              created_at: '2026-10-01T00:00:00Z',
            },
          ]}
        />
      </Providers>,
    )
    expect(screen.getByTestId('page-banner-compact')).toBeTruthy()
    expect(screen.getAllByText('media-nas').length).toBeGreaterThan(0)
    expect(
      screen.getAllByRole('button', { name: /add network share/i }),
    ).toHaveLength(1)
  })
})
