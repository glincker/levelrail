import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { GitSshKeySection } from './GitSshKeySection'
import commonEn from '../locales/en/common.json'
import gitSourceEn from '../locales/en/gitSource.json'
import { isSshRepoUrl } from '../lib/gitSshUrl'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@/hooks/useAbilities', () => ({
  useAbilities: () => ({ can: () => true }),
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'gitSource'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, gitSource: gitSourceEn } },
  interpolation: { escapeValue: false },
})

function reply(status: number, body?: unknown): Promise<Response> {
  return Promise.resolve(
    new Response(body === undefined ? null : JSON.stringify(body), {
      status,
      headers: { 'Content-Type': 'application/json' },
    }),
  )
}

function mount(handler: (init?: RequestInit) => Promise<Response>) {
  const fetchMock = vi.fn((_u: RequestInfo | URL, init?: RequestInit) =>
    handler(init),
  )
  vi.stubGlobal('fetch', fetchMock)
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={createTestQueryClient()}>
        <GitSshKeySection appName="web" />
      </QueryClientProvider>
    </I18nextProvider>,
  )
  return fetchMock
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('isSshRepoUrl', () => {
  it('accepts ssh and scp-like urls only', () => {
    expect(isSshRepoUrl('ssh://git@host/org/r.git')).toBe(true)
    expect(isSshRepoUrl('git@github.com:org/r.git')).toBe(true)
    expect(isSshRepoUrl('https://github.com/org/r.git')).toBe(false)
  })
})

describe('GitSshKeySection', () => {
  it('generates with a pasted host key and shows the server error on 400', async () => {
    const fetchMock = mount((init) =>
      init?.method === 'POST'
        ? reply(400, { error: 'host_key must be public keys' })
        : reply(404, { error: 'none' }),
    )
    expect(await screen.findByText('No key yet')).toBeTruthy()
    const generate = screen.getByRole('button', { name: /generate key/i })
    expect((generate as HTMLButtonElement).disabled).toBe(true)
    fireEvent.change(screen.getByLabelText('Host key'), {
      target: { value: 'host ssh-ed25519 AAAA' },
    })
    fireEvent.click(generate)
    expect(await screen.findByRole('alert')).toBeTruthy()
    const post = fetchMock.mock.calls.find((c) => c[1]?.method === 'POST')
    expect(JSON.parse(String(post?.[1]?.body as string))).toEqual({
      host_key: 'host ssh-ed25519 AAAA',
    })
  })

  it('shows the key and confirms before removing', async () => {
    const fetchMock = mount((init) =>
      init?.method === 'DELETE'
        ? reply(204)
        : reply(200, {
            public_key: 'ssh-ed25519 AAAAkey',
            fingerprint: 'SHA256:abc',
            host_key_pinned: true,
            host_fingerprint: 'SHA256:host',
          }),
    )
    expect(await screen.findByText('ssh-ed25519 AAAAkey')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: /^remove$/i }))
    fireEvent.click(await screen.findByRole('button', { name: /remove key/i }))
    await waitFor(() =>
      expect(fetchMock.mock.calls.some((c) => c[1]?.method === 'DELETE')).toBe(
        true,
      ),
    )
  })
})
