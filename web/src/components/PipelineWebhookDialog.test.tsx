import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { PipelinesI18n } from '../test/pipelinesI18n'
import type { Pipeline } from '../types/pipelines'
import { PipelineWebhookDialog } from './PipelineWebhookDialog'
import { createTestQueryClient } from '@/test/testQueryClient'

const pipeline: Pipeline = {
  id: 'p1',
  app: 'web',
  name: 'ci',
  source: 'ui',
  enabled: true,
  triggers: ['webhook'],
  jobs: 1,
  created_at: '2026-10-16T10:00:00Z',
  updated_at: '2026-10-16T10:00:00Z',
}

function jsonResponse(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('PipelineWebhookDialog', () => {
  it('creates a signed webhook and shows the token and secret once', async () => {
    const fetchMock = vi.fn<
      (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
    >((_url, init) => {
      if (init?.method === 'PUT') {
        return Promise.resolve(
          jsonResponse({
            configured: true,
            enabled: true,
            hmac: true,
            path: '/api/v1/hooks/pipeline/p1/the-token',
            token: 'the-token',
            secret: 'the-secret',
          }),
        )
      }
      return Promise.resolve(jsonResponse({ configured: false }))
    })
    vi.stubGlobal('fetch', fetchMock)
    const qc = createTestQueryClient()
    render(
      <QueryClientProvider client={qc}>
        <PipelinesI18n>
          <PipelineWebhookDialog appName="web" pipeline={pipeline} />
        </PipelinesI18n>
      </QueryClientProvider>,
    )
    await userEvent.click(screen.getByRole('button', { name: 'Webhook' }))
    expect(await screen.findByText('No webhook trigger yet.')).toBeVisible()
    await userEvent.click(
      screen.getByRole('switch', { name: 'Require a signature' }),
    )
    await userEvent.click(
      screen.getByRole('button', { name: 'Create webhook' }),
    )

    await waitFor(() => expect(screen.getByText('the-secret')).toBeVisible())
    expect(
      screen.getByText(/\/api\/v1\/hooks\/pipeline\/p1\/the-token/),
    ).toBeVisible()
    expect(screen.getByText(/Shown once/)).toBeVisible()
    const put = fetchMock.mock.calls.find(([, init]) => init?.method === 'PUT')
    expect(put?.[0]).toBe('/api/v1/apps/web/pipelines/ci/webhook')
    expect(JSON.parse(put?.[1]?.body as string)).toEqual({
      hmac: true,
      enabled: true,
    })
  })
})
