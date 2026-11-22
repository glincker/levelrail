import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AppEnvCompareCard } from './AppEnvCompareCard'
import type { AppDetail } from '../types/appDetail'

vi.mock('../queries/apps', () => ({
  useAppListOptional: () => ({ data: [{ name: 'web' }, { name: 'api' }] }),
}))

describe('AppEnvCompareCard', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('lists differing keys and never renders a value', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          app: 'web',
          against: 'api',
          added: ['NEW_KEY'],
          removed: [],
          changed: ['PORT'],
          same: 3,
          secret_unchecked: 1,
        }),
        { status: 200 },
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    render(
      <QueryClientProvider client={new QueryClient()}>
        <AppEnvCompareCard app={{ name: 'web' } as AppDetail} />
      </QueryClientProvider>,
    )
    await userEvent.selectOptions(screen.getByRole('combobox'), 'api')
    await waitFor(() => expect(screen.getByText('NEW_KEY')).toBeTruthy())
    expect(screen.getByText('PORT')).toBeTruthy()
    expect(String(fetchMock.mock.calls[0]?.[0])).toContain(
      '/api/v1/apps/web/env-diff?against=api',
    )
  })
})
