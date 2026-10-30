import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import gitBuildEn from '../locales/en/gitBuild.json'
import { GitPushRemoteCard } from './GitPushRemoteCard'
import type { GitRemoteResource } from '../types/gitRemote'
import { createTestQueryClient } from '@/test/testQueryClient'

const features: { list: string[] } = { list: ['git-push'] }
vi.mock('../hooks/useExperimental', () => ({
  useExperimentalFeatures: () => features.list,
}))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => (
    <a href="/x">{children}</a>
  ),
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['gitBuild'],
  defaultNS: 'gitBuild',
  resources: { en: { gitBuild: gitBuildEn } },
  interpolation: { escapeValue: false },
})

function remote(over: Partial<GitRemoteResource> = {}): GitRemoteResource {
  return {
    configured: false,
    url: 'https://cp.example.com/api/v1/git/web.git',
    remote_name: 'deploy',
    username: 'git',
    branch: 'main',
    max_push_bytes: 268435456,
    ...over,
  }
}

function renderCard() {
  const client = createTestQueryClient()
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={client}>
        <GitPushRemoteCard appName="web" />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('GitPushRemoteCard', () => {
  const fetchMock = vi.fn()
  beforeEach(() => {
    features.list = ['git-push']
    vi.stubGlobal('fetch', fetchMock)
  })
  afterEach(() => {
    fetchMock.mockReset()
    vi.unstubAllGlobals()
  })

  it('points at Beta features while the feature is off', () => {
    features.list = []
    renderCard()
    expect(screen.getByText(/switched off/i)).toBeTruthy()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('enables the remote and shows the one-time command', async () => {
    let configured = false
    fetchMock.mockImplementation((_url: string, init?: RequestInit) => {
      if (init?.method === 'POST') {
        configured = true
        return Promise.resolve(
          new Response(
            JSON.stringify(
              remote({
                configured: true,
                token: 'gp_abc',
                add_command:
                  'git remote add deploy https://git:gp_abc@cp.example.com/api/v1/git/web.git',
              }),
            ),
          ),
        )
      }
      return Promise.resolve(
        new Response(JSON.stringify(remote({ configured }))),
      )
    })
    renderCard()
    await userEvent.click(
      await screen.findByRole('button', { name: 'Enable git push' }),
    )
    expect(await screen.findByText(/gp_abc@cp.example.com/)).toBeTruthy()
    await waitFor(() => {
      expect(screen.getByText(/shown once/i)).toBeTruthy()
    })
  })

  it('shows the remote line without a token once configured', async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify(remote({ configured: true }))),
    )
    renderCard()
    expect(
      await screen.findByText(
        'git remote add deploy https://cp.example.com/api/v1/git/web.git',
      ),
    ).toBeTruthy()
    expect(screen.getByRole('button', { name: /rotate token/i })).toBeTruthy()
  })
})
