import type { ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import clustersEn from '../../locales/en/clusters.json'
import commonEn from '../../locales/en/common.json'
import { parseEnvLines } from '../../lib/envLines'
import { DeployForm } from './DeployForm'
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

describe('parseEnvLines', () => {
  it('reports the line number, never the value', () => {
    const r = parseEnvLines('A=1\nsecret-value-only')
    expect(r).toEqual({ ok: false, line: 2 })
  })
})

describe('DeployForm', () => {
  it('posts env and clears it so values are not shown again', async () => {
    const fetchMock = vi.fn((_url: string, init?: RequestInit) => {
      if (init?.method === 'POST') {
        return Promise.resolve(
          new Response(JSON.stringify({ app: 'web' }), { status: 200 }),
        )
      }
      return Promise.resolve(
        new Response(JSON.stringify({ settings: [], restart: {} }), {
          status: 200,
        }),
      )
    })
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()
    wrap(<DeployForm cluster="c1" />)

    await user.type(screen.getByLabelText('App name'), 'web')
    await user.type(screen.getByLabelText('Image'), 'nginx:1.27')
    await user.type(
      screen.getByLabelText('Environment (optional)'),
      'TOKEN=hunter2',
    )
    await user.click(screen.getByRole('button', { name: 'Deploy' }))

    await waitFor(() => {
      expect(screen.getByLabelText('Environment (optional)')).toHaveValue('')
    })
    expect(screen.queryByText(/hunter2/)).toBeNull()
    const post = fetchMock.mock.calls.find((c) => c[1]?.method === 'POST')
    const body = post?.[1]?.body
    expect(typeof body === 'string' ? body : '').toContain('"TOKEN":"hunter2"')
  })

  it('shows a 409 refusal as a readable message', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((_u: string, init?: RequestInit) =>
        Promise.resolve(
          init?.method === 'POST'
            ? new Response(JSON.stringify({ error: 'host already used' }), {
                status: 409,
                headers: { 'Content-Type': 'application/json' },
              })
            : new Response(JSON.stringify({ settings: [] }), { status: 200 }),
        ),
      ),
    )
    const user = userEvent.setup()
    wrap(<DeployForm cluster="c1" />)
    await user.type(screen.getByLabelText('App name'), 'web')
    await user.type(screen.getByLabelText('Image'), 'nginx')
    await user.click(screen.getByRole('button', { name: 'Deploy' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'host already used',
    )
  })

  it('warns on a registry mismatch and shows the backend 400 readably', async () => {
    const msg =
      'the image is on registry docker.io but pull secret gh is for ghcr.io; nothing was sent'
    const fetchMock = vi.fn((u: string, init?: RequestInit) => {
      if (init?.method === 'POST') {
        return Promise.resolve(
          new Response(JSON.stringify({ error: msg }), {
            status: 400,
            headers: { 'Content-Type': 'application/json' },
          }),
        )
      }
      if (u.endsWith('/pull-secrets')) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              pull_secrets: [{ name: 'gh', registry: 'ghcr.io' }],
            }),
            { status: 200 },
          ),
        )
      }
      return Promise.resolve(
        new Response(JSON.stringify({ settings: [] }), { status: 200 }),
      )
    })
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()
    wrap(<DeployForm cluster="c1" />)
    await user.type(screen.getByLabelText('App name'), 'web')
    await user.type(screen.getByLabelText('Image'), 'index.docker.io/acme/web')
    await user.selectOptions(
      await screen.findByLabelText('Pull secret (optional)'),
      'gh',
    )
    expect(
      screen.getByText('The image is on docker.io but gh is for ghcr.io.'),
    ).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Deploy' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(msg)
  })
})
