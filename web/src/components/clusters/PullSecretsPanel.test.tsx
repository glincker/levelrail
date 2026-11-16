import type { ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import clustersEn from '../../locales/en/clusters.json'
import commonEn from '../../locales/en/common.json'
import { PullSecretsPanel } from './PullSecretsPanel'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'clusters'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, clusters: clustersEn } },
  interpolation: { escapeValue: false },
})

function wrap(node: ReactNode) {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <I18nextProvider i18n={i18n}>{node}</I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('PullSecretsPanel', () => {
  it('lists names and hosts, then clears the password after save', async () => {
    const fetchMock = vi.fn((_url: string, init?: RequestInit) => {
      if (init?.method === 'POST') {
        return Promise.resolve(
          new Response(JSON.stringify({ name: 'gh', registry: 'ghcr.io' }), {
            status: 201,
          }),
        )
      }
      return Promise.resolve(
        new Response(
          JSON.stringify({
            pull_secrets: [{ name: 'hub', registry: 'docker.io' }],
          }),
          { status: 200 },
        ),
      )
    })
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()
    wrap(<PullSecretsPanel cluster="prod" />)
    expect(await screen.findByText('docker.io')).toBeTruthy()

    await user.type(screen.getByLabelText('Name'), 'gh')
    await user.type(screen.getByLabelText('Registry host'), 'ghcr.io')
    await user.type(screen.getByLabelText('Username'), 'me')
    await user.type(screen.getByLabelText('Password or token'), 'hunter2')
    await user.click(screen.getByRole('button', { name: 'Add' }))

    await waitFor(() => {
      expect(
        screen.getByLabelText<HTMLInputElement>('Password or token').value,
      ).toBe('')
    })
    const post = fetchMock.mock.calls.find(([, i]) => i?.method === 'POST')
    expect(post?.[0]).toBe('/api/v1/clusters/prod/pull-secrets')
    expect(document.body.textContent).not.toContain('hunter2')
  })

  it('requires the typed name before deleting', async () => {
    const fetchMock = vi.fn((_url: string, init?: RequestInit) =>
      Promise.resolve(
        init?.method === 'DELETE'
          ? new Response(null, { status: 204 })
          : new Response(
              JSON.stringify({
                pull_secrets: [{ name: 'hub', registry: 'docker.io' }],
              }),
              { status: 200 },
            ),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()
    wrap(<PullSecretsPanel cluster="prod" />)
    await user.click(await screen.findByRole('button', { name: 'Delete hub' }))
    const confirm = await screen.findByRole('button', {
      name: 'Delete secret',
    })
    expect((confirm as HTMLButtonElement).disabled).toBe(true)
    await user.type(screen.getByRole('textbox'), 'hub')
    expect((confirm as HTMLButtonElement).disabled).toBe(false)
    await user.click(confirm)
    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some(
          ([u, i]) =>
            u === '/api/v1/clusters/prod/pull-secrets/hub' &&
            i?.method === 'DELETE',
        ),
      ).toBe(true)
    })
  })
})
