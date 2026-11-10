import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider, type QueryClient } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ComposeSourceCard from './ComposeSourceCard'
import { AllI18n } from '../test/allI18n'
import { createTestQueryClient } from '@/test/testQueryClient'
import { composeSourceKeys } from '../queries/composeSource'

const SECRET = 'whsec-test-only'
const stack = { app_id: 'stack', phase: 'running', services: [] }
const resource = {
  service_name: 'stack',
  repo_url: 'https://github.com/acme/stack.git',
  branch: 'main',
  build_type: 'dockerfile',
  compose_path: 'compose.yaml',
  trigger_mode: 'push',
  has_token: false,
  webhook_url: '/webhooks/git/stack',
  preview_enabled: false,
  post_pr_comments: false,
  deploy_paths: [],
  deploy_paths_ignore: [],
  report_status: false,
  created_at: '2026-11-09T00:00:00Z',
  updated_at: '2026-11-09T00:00:00Z',
}

function json(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

let putResponse: () => Response
let client: QueryClient

beforeEach(() => {
  putResponse = () => json(201, { ...resource, webhook_secret: SECRET })
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url =
        typeof input === 'string'
          ? input
          : input instanceof URL
            ? input.href
            : input.url
      const method = init?.method ?? 'GET'
      if (url === '/api/v1/apps/stack/compose') {
        return Promise.resolve(json(200, stack))
      }
      if (url === '/api/v1/apps/stack/compose-source' && method === 'GET') {
        return Promise.resolve(json(404, { error: 'not connected' }))
      }
      if (url === '/api/v1/apps/stack/compose-source' && method === 'PUT') {
        return Promise.resolve(putResponse())
      }
      return Promise.resolve(json(500, { error: 'unexpected' }))
    }),
  )
  client = createTestQueryClient()
  render(
    <AllI18n>
      <QueryClientProvider client={client}>
        <ComposeSourceCard appName="stack" />
      </QueryClientProvider>
    </AllI18n>,
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
})

async function fillAndSubmit() {
  const user = userEvent.setup()
  await user.type(
    await screen.findByLabelText('Repository URL'),
    'https://github.com/acme/stack.git',
  )
  await user.click(screen.getByRole('button', { name: 'Connect repository' }))
  return user
}

describe('ComposeSourceCard', () => {
  it('blocks a path outside the repository before sending', async () => {
    const user = userEvent.setup()
    const path = await screen.findByLabelText('Compose file')
    await user.clear(path)
    await user.type(path, '../compose.yaml')
    expect(
      screen.getByText('The path must stay inside the repository (no ..).'),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Connect repository' }),
    ).toBeDisabled()
  })

  it('shows the webhook secret once, keeps it out of the cache, clears it on done', async () => {
    const user = await fillAndSubmit()
    expect(await screen.findByText(SECRET)).toBeInTheDocument()
    const cached = client.getQueryData<{ webhook_secret?: string }>(
      composeSourceKeys.detail('stack'),
    )
    expect(cached?.webhook_secret).toBeUndefined()
    await user.click(screen.getByRole('button', { name: 'Done, I copied it' }))
    expect(screen.queryByText(SECRET)).not.toBeInTheDocument()
  })

  it('explains a 409 conflict with a single-service source', async () => {
    putResponse = () =>
      json(409, {
        error: 'this app already has a git source of the other kind',
      })
    await fillAndSubmit()
    expect(
      await screen.findByText(/already deploys a single service from git/),
    ).toBeInTheDocument()
  })

  it('explains a 423 freeze', async () => {
    putResponse = () => json(423, { error: 'deploys are frozen' })
    await fillAndSubmit()
    await waitFor(() =>
      expect(screen.getByText(/Deploys are frozen/)).toBeInTheDocument(),
    )
  })
})
