import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { GitDeploySettingsCard } from './GitDeploySettingsCard'

function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function urlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.toString()
  return input.url
}

const source = {
  service_name: 'web',
  repo_url: 'https://github.com/o/r',
  branch: 'main',
  build_type: 'dockerfile',
  trigger_mode: 'push',
  has_token: false,
  webhook_url: '/hook',
  preview_enabled: false,
  post_pr_comments: false,
  deploy_paths: ['src/**'],
  deploy_paths_ignore: [],
  report_status: true,
  created_at: '',
  updated_at: '',
}

function renderCard() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <GitDeploySettingsCard appName="web" />
    </QueryClientProvider>,
  )
}

describe('GitDeploySettingsCard', () => {
  let puts: unknown[]
  let connected: boolean

  beforeEach(() => {
    puts = []
    connected = true
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = urlOf(input)
        if (url.endsWith('/deploy-settings') && init?.method === 'PUT') {
          const body = JSON.parse(init.body as string) as Record<
            string,
            unknown
          >
          puts.push(body)
          return Promise.resolve(jsonResponse({ ...body }))
        }
        if (url.endsWith('/git-source')) {
          return Promise.resolve(
            connected ? jsonResponse(source) : jsonResponse({}, 404),
          )
        }
        return Promise.resolve(jsonResponse({}, 404))
      }),
    )
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders nothing when no git source is connected', async () => {
    connected = false
    renderCard()
    await waitFor(() => expect(fetch).toHaveBeenCalled())
    expect(screen.queryByText(/Deploy filters/)).toBeNull()
  })

  it('saves edited globs and the reporting switch', async () => {
    const user = userEvent.setup()
    renderCard()
    const ignore = await screen.findByRole('textbox', {
      name: /Skip when only these change/,
    })
    const save = screen.getByRole('button', { name: 'Save' })
    expect((save as HTMLButtonElement).disabled).toBe(true)

    await user.type(ignore, 'docs/**{Enter}**/*.md')
    await user.click(screen.getByRole('switch'))
    await user.click(save)

    await waitFor(() => expect(puts).toHaveLength(1))
    expect(puts[0]).toEqual({
      deploy_paths: ['src/**'],
      deploy_paths_ignore: ['docs/**', '**/*.md'],
      report_status: false,
    })
  })
})
