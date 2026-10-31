import { describe, expect, it, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { AllI18n } from '../test/allI18n'
import { StatusWebhooksPanel } from './StatusWebhooksPanel'
import { createTestQueryClient } from '@/test/testQueryClient'

const SECRET = 'whsec_testsecretvalue'

function mockApi() {
  const hooks: Record<string, unknown>[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((input: string, init?: RequestInit) => {
      const method = init?.method ?? 'GET'
      if (method === 'POST' && input.endsWith('/webhooks')) {
        const body = JSON.parse(
          typeof init?.body === 'string' ? init.body : '{}',
        ) as { url: string }
        const hook = {
          id: 'swh_1',
          url: body.url,
          label: '',
          events: [],
          paused: false,
          consecutive_failures: 0,
          created_at: '2026-10-30T00:00:00Z',
        }
        hooks.push(hook)
        return Promise.resolve(
          new Response(JSON.stringify({ ...hook, signing_secret: SECRET }), {
            status: 201,
          }),
        )
      }
      return Promise.resolve(
        new Response(JSON.stringify(hooks), { status: 200 }),
      )
    }),
  )
}

function renderPanel() {
  const client = createTestQueryClient()
  return render(
    <QueryClientProvider client={client}>
      <AllI18n>
        <StatusWebhooksPanel />
      </AllI18n>
    </QueryClientProvider>,
  )
}

describe('StatusWebhooksPanel', () => {
  beforeEach(() => {
    mockApi()
  })

  it('shows the signing secret once and never in the list', async () => {
    renderPanel()
    const url = await screen.findByLabelText('Webhook URL')
    fireEvent.change(url, { target: { value: 'https://example.com/hook' } })
    fireEvent.click(screen.getByRole('button', { name: /Add webhook/ }))
    expect(await screen.findByText(SECRET)).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'I saved it' }))
    await waitFor(() => expect(screen.queryByText(SECRET)).toBeNull())
    expect(
      (await screen.findAllByText('https://example.com/hook')).length,
    ).toBeGreaterThan(0)
  })
})
