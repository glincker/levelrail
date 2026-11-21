import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AppDetail } from '../types/appDetail'
import { createTestQueryClient } from '@/test/testQueryClient'
import { LogDrainCard } from './LogDrainCard'

function renderCard(drain?: AppDetail['log_drain']) {
  const fetchMock = vi.fn((_input: RequestInfo | URL, init?: RequestInit) =>
    Promise.resolve({
      ok: true,
      status: 200,
      json: () =>
        Promise.resolve(
          init?.body
            ? {
                app_name: 'web',
                ...(JSON.parse(init.body as string) as object),
                header_value: undefined,
                header_set: true,
              }
            : { id: 'x', fields: [] },
        ),
    } as unknown as Response),
  )
  vi.stubGlobal('fetch', fetchMock)
  const app = { name: 'web', log_drain: drain } as unknown as AppDetail
  render(
    <QueryClientProvider client={createTestQueryClient()}>
      <LogDrainCard app={app} />
    </QueryClientProvider>,
  )
  return fetchMock
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('LogDrainCard auth header', () => {
  it('sends the header name and value and keeps the value out of the page', async () => {
    const fetchMock = renderCard()
    const user = userEvent.setup()

    await user.type(
      screen.getByPlaceholderText(/collector.example.com/),
      'https://c.example.com/in',
    )
    await user.click(screen.getByRole('button', { name: /advanced/i }))
    await user.type(
      await screen.findByPlaceholderText('Authorization'),
      'Authorization',
    )
    await user.type(
      document.getElementById('log-drain-header-value') as HTMLElement,
      'Bearer s3cret',
    )
    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      const put = fetchMock.mock.calls.find(
        ([, init]) => init?.method === 'PUT',
      )
      expect(put).toBeDefined()
      const body = JSON.parse((put?.[1] as RequestInit).body as string) as {
        header_name: string
        header_value: string
      }
      expect(body.header_name).toBe('Authorization')
      expect(body.header_value).toBe('Bearer s3cret')
    })
    await waitFor(() => {
      expect(document.body.textContent).not.toContain('s3cret')
    })
  })
})
