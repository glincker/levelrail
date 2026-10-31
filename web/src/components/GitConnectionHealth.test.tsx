import { Suspense } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { GitConnectionBanner, GitConnectionHealth } from './GitConnectionHealth'
import commonEn from '../locales/en/common.json'
import gitReposEn from '../locales/en/gitRepos.json'
import type { GitProviderStatus } from '../types/gitProviders'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'gitRepos'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, gitRepos: gitReposEn } },
  interpolation: { escapeValue: false },
})

function provider(
  name: GitProviderStatus['provider'],
  connection: GitProviderStatus['connection'],
): GitProviderStatus {
  return {
    provider: name,
    connected: true,
    can_list_branches: true,
    can_register_webhook: true,
    can_auth_clone: false,
    connection,
  }
}

function wrap(node: React.ReactNode, providers?: GitProviderStatus[]) {
  if (providers) {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          new Response(JSON.stringify(providers), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          }),
        ),
      ),
    )
  }
  const queryClient = createTestQueryClient()
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <Suspense fallback={<div>loading</div>}>{node}</Suspense>
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('GitConnectionBanner', () => {
  it('shows nothing for a healthy connection', () => {
    wrap(
      <GitConnectionBanner provider={provider('gitlab', { status: 'ok' })} />,
    )
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('offers a primary Reconnect button that starts the provider authorization', () => {
    const assign = vi.fn()
    vi.stubGlobal('location', { ...window.location, assign })
    wrap(
      <GitConnectionBanner
        provider={provider('gitlab', {
          status: 'needs_reauthorize',
          detail: 'the authorization was revoked or has expired',
        })}
      />,
    )

    expect(
      screen.getByText('GitLab needs to be reconnected'),
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Reconnect GitLab' }))
    expect(assign).toHaveBeenCalledWith('/settings/gitlab-app')
  })

  it('sends a removed GitHub installation to the App settings page', () => {
    const assign = vi.fn()
    vi.stubGlobal('location', { ...window.location, assign })
    wrap(
      <GitConnectionBanner
        provider={provider('github', { status: 'revoked' })}
      />,
    )

    expect(
      screen.getByText('The GitHub connection was removed'),
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Reconnect GitHub' }))
    expect(assign).toHaveBeenCalledWith('/settings/github-app')
  })

  it('says a rate limited connection recovers by itself, without a Reconnect button', () => {
    wrap(
      <GitConnectionBanner
        provider={provider('gitea', {
          status: 'rate_limited',
          next_check_at: '2026-10-20T10:05:00Z',
        })}
      />,
    )
    expect(
      screen.getByText('Gitea is rate limiting this control plane'),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /Reconnect/ }),
    ).not.toBeInTheDocument()
  })
})

describe('GitConnectionHealth', () => {
  it('shows a healthy line on the settings card', async () => {
    wrap(<GitConnectionHealth provider="gitlab" />, [
      provider('gitlab', { status: 'ok' }),
    ])
    expect(
      await screen.findByText('Connection health: Healthy'),
    ).toBeInTheDocument()
  })

  it('shows the Reconnect banner when the connection needs it', async () => {
    wrap(<GitConnectionHealth provider="bitbucket" />, [
      provider('bitbucket', { status: 'needs_reauthorize' }),
    ])
    expect(
      await screen.findByRole('button', { name: 'Reconnect Bitbucket' }),
    ).toBeInTheDocument()
  })
})
