import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import previewsEn from '../locales/en/previews.json'
import { PrCommentCommandsSetting } from './PrCommentCommandsSetting'

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

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['previews'],
  defaultNS: 'previews',
  resources: { en: { previews: previewsEn } },
  interpolation: { escapeValue: false },
})

const baseSource = {
  service_name: 'web',
  repo_url: 'https://github.com/org/web.git',
  branch: 'main',
  build_type: 'railpack',
  has_token: false,
  webhook_url: '/api/v1/webhooks/github/web',
  preview_enabled: false,
  post_pr_comments: false,
  pr_comment_commands: false,
  pr_comment_commands_forks: false,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}

function renderSetting() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <PrCommentCommandsSetting appName="web" />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('PrCommentCommandsSetting', () => {
  let source: typeof baseSource
  let puts: unknown[]

  beforeEach(() => {
    source = { ...baseSource }
    puts = []
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = urlOf(input)
        if (url.endsWith('/preview-settings') && init?.method === 'PUT') {
          const body = JSON.parse(init.body as string) as Record<
            string,
            boolean
          >
          puts.push(body)
          source = { ...source, ...body }
          return Promise.resolve(jsonResponse(source))
        }
        return Promise.resolve(jsonResponse(source))
      }),
    )
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('is off by default and hides the fork switch', async () => {
    renderSetting()
    const sw = await screen.findByRole('switch', {
      name: 'Pull request comment commands enabled',
    })
    expect(sw).toHaveAttribute('aria-checked', 'false')
    expect(
      screen.queryByRole('switch', { name: /fork pull requests/i }),
    ).toBeNull()
  })

  it('sends only the commands flag when enabled, then offers the fork switch', async () => {
    const user = userEvent.setup()
    renderSetting()
    await user.click(
      await screen.findByRole('switch', {
        name: 'Pull request comment commands enabled',
      }),
    )
    await waitFor(() => {
      expect(puts).toEqual([{ pr_comment_commands: true }])
    })
    expect(
      await screen.findByRole('switch', {
        name: 'Pull request comment commands on fork pull requests enabled',
      }),
    ).toHaveAttribute('aria-checked', 'false')
  })
})
