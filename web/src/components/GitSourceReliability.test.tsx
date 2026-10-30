import { Suspense } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { GitSourceReliability } from './GitSourceReliability'
import commonEn from '../locales/en/common.json'
import fixitEn from '../locales/en/fixit.json'
import gitReposEn from '../locales/en/gitRepos.json'
import type { GitSourceResource } from '../types/gitSource'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('../hooks/useIsRoot', () => ({ useIsRoot: () => true }))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'gitRepos', 'fixit'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, gitRepos: gitReposEn, fixit: fixitEn } },
  interpolation: { escapeValue: false },
})

function source(extra: Partial<GitSourceResource> = {}): GitSourceResource {
  return {
    service_name: 'web',
    repo_url: 'https://github.com/org/web.git',
    branch: 'main',
    build_type: 'dockerfile',
    trigger_mode: 'push',
    has_token: false,
    webhook_url: '/api/v1/webhooks/github/web',
    preview_enabled: false,
    post_pr_comments: false,
    deploy_paths: [],
    deploy_paths_ignore: [],
    report_status: true,
    created_at: '2026-10-20T10:00:00Z',
    updated_at: '2026-10-20T10:00:00Z',
    ...extra,
  }
}

function json(body: unknown): Promise<Response> {
  return Promise.resolve(
    new Response(JSON.stringify(body), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    }),
  )
}

function renderPanel(
  s: GitSourceResource,
  handler: (url: string, init?: RequestInit) => Promise<Response>,
) {
  const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) =>
    handler(
      typeof input === 'string' ? input : (input as URL).toString(),
      init,
    ),
  )
  vi.stubGlobal('fetch', fetchMock)
  const queryClient = createTestQueryClient()
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <Suspense fallback={<div>loading</div>}>
          <GitSourceReliability appName="web" source={s} />
        </Suspense>
      </QueryClientProvider>
    </I18nextProvider>,
  )
  return fetchMock
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('GitSourceReliability', () => {
  it('warns about a private webhook address with a Fix action, not a command', async () => {
    renderPanel(source({ webhook_url_warning: 'private_address' }), (url) => {
      if (url.startsWith('/api/v1/fixes')) {
        return json({
          fixes: [
            {
              id: 'settings.webhook_url.public',
              check_id: 'webhook_public_url',
              subject: 'settings',
              kind: 'auto',
              title: 'Set a public URL for git webhooks',
              why: 'Git hosts call this control plane.',
              risk: 'safe',
              reversible: true,
              state: 'offer',
              failing: true,
              detail:
                'git webhook URLs point at an address a git host cannot reach',
              changes: [],
              param: { name: 'url', options: [], free: true },
              attempts: [],
            },
          ],
        })
      }
      return json({ fixes: [] })
    })

    expect(
      await screen.findByText(/webhook address is private or local/),
    ).toBeInTheDocument()
    expect(
      await screen.findByText('Set a public URL for git webhooks'),
    ).toBeInTheDocument()
  })

  it('shows a failed registration with the error and a Register again button that retries', async () => {
    const fetchMock = renderPanel(
      source({
        webhook_registration: { state: 'failed', error: 'permission denied' },
      }),
      (url, init) => {
        if (init?.method === 'POST' && url.endsWith('/webhook/register')) {
          return json(source({ webhook_registration: { state: 'registered' } }))
        }
        return json({ fixes: [] })
      },
    )

    expect(
      screen.getByText(/was not registered on the provider: permission denied/),
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Register again' }))
    await waitFor(() => {
      const posted = fetchMock.mock.calls.some(
        ([u, i]) =>
          typeof u === 'string' &&
          u.endsWith('/api/v1/apps/web/git-source/webhook/register') &&
          i?.method === 'POST',
      )
      expect(posted).toBe(true)
    })
  })

  it('shows when the poller and webhooks were last heard from, and a poll failure', () => {
    renderPanel(
      source({
        webhook_registration: { state: 'registered' },
        poll: {
          enabled: true,
          last_polled_at: '2026-10-20T10:00:00Z',
          error: 'host unreachable',
        },
      }),
      () => json({ fixes: [] }),
    )

    expect(
      screen.getByText('Webhook registered on the provider.'),
    ).toBeInTheDocument()
    expect(screen.getByText(/Last checked/)).toHaveTextContent(
      'No webhook has arrived yet.',
    )
    expect(
      screen.getByText(/The last check failed: host unreachable/),
    ).toBeInTheDocument()
  })

  it('says needs reconnect with a Reconnect action when the source account was disconnected', () => {
    renderPanel(
      source({
        repo_url: 'https://git.corp.example/g/web.git',
        connection_id: 'gc_gone',
        connection: { id: 'gc_gone', state: 'needs_reconnect' },
      }),
      () => json({}),
    )
    expect(
      screen.getByText(
        /The account this repository was picked from was disconnected/,
      ),
    ).toBeInTheDocument()
    expect(screen.getByText('Reconnect GitLab').closest('a')).toHaveAttribute(
      'href',
      '/settings/gitlab-app',
    )
    expect(screen.getByText('Reconnect Gitea')).toBeInTheDocument()
  })

  it('names the account whose authorization was revoked', () => {
    renderPanel(
      source({
        connection_id: 'gc_1',
        connection: {
          id: 'gc_1',
          provider: 'gitlab',
          label: 'Work',
          state: 'revoked',
        },
      }),
      () => json({}),
    )
    expect(
      screen.getByText(/The account Work was removed on the provider/),
    ).toBeInTheDocument()
    expect(screen.queryByText('Reconnect Gitea')).not.toBeInTheDocument()
  })
})
