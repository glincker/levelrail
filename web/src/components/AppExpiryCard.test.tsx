import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { NewSurfacesProviders } from '../test/NewSurfacesProviders'
import { jsonResponse } from '../test/newSurfacesI18n'
import { AppExpiryCard } from './AppExpiryCard'

vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('AppExpiryCard', () => {
  it('shows the kept reason and a clear button for a stopped app', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          jsonResponse({
            app: 'web',
            expiry: {
              expires_at: '2026-12-01T18:00:00Z',
              delete_data: false,
              stopped_at: '2026-12-01T18:05:00Z',
              kept_reason: 'app has persistent volumes',
            },
            state: 'kept',
            grace_hours: 168,
            notice_hours: 24,
            max_horizon_days: 90,
            delete_enabled: false,
          }),
        ),
      ),
    )
    render(
      <NewSurfacesProviders>
        <AppExpiryCard appName="web" />
      </NewSurfacesProviders>,
    )
    expect(
      await screen.findByText(/app has persistent volumes/),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Clear expiry' })).toBeEnabled()
    expect(screen.getByText(/Delete disabled by operator/)).toBeInTheDocument()
  })
})
