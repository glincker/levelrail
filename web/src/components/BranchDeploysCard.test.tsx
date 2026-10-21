import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import branchDeploysEn from '../locales/en/branchDeploys.json'
import { BranchDeploysCard } from './BranchDeploysCard'
import type { BranchDeploys } from '../types/branchDeploy'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['branchDeploys'],
  defaultNS: 'branchDeploys',
  resources: { en: { branchDeploys: branchDeploysEn } },
  interpolation: { escapeValue: false },
})

function urlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.toString()
  return input.url
}

function bodyOf(init?: RequestInit): string {
  return typeof init?.body === 'string' ? init.body : ''
}

function fakeJson(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function fixture(overrides: Partial<BranchDeploys> = {}): BranchDeploys {
  return {
    settings: {
      mode: 'patterns',
      patterns: ['staging'],
      production_branch: 'main',
      idle_ttl_hours: 336,
    },
    deploys: [
      {
        branch: 'staging',
        slug: 'staging',
        deploy_app_id: 'web-br-staging',
        url: 'https://web-br-staging.example.com',
        head_sha: '0123456789abcdef',
        head_message: 'fix login',
        status: 'active',
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
      },
    ],
    ...overrides,
  }
}

function renderCard() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <BranchDeploysCard appName="web" />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('BranchDeploysCard', () => {
  let fetchMock: ReturnType<typeof vi.fn>
  let current: BranchDeploys

  beforeEach(() => {
    current = fixture()
    fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = urlOf(input)
      const method = init?.method ?? 'GET'
      if (method === 'PUT') {
        const body = JSON.parse(bodyOf(init)) as {
          mode: 'off' | 'all' | 'patterns'
          patterns: string[]
        }
        current = {
          ...current,
          settings: { ...current.settings, ...body },
        }
        return Promise.resolve(fakeJson(current))
      }
      if (method === 'DELETE') {
        current = { ...current, deploys: [] }
        return Promise.resolve(fakeJson({}, 204))
      }
      if (url.endsWith('/branch-deploys')) {
        return Promise.resolve(fakeJson(current))
      }
      return Promise.resolve(fakeJson({}, 404))
    })
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('lists the live deploy with its URL, commit and an open link', async () => {
    renderCard()
    expect(
      await screen.findByText('staging', { selector: 'span' }),
    ).toBeTruthy()
    expect(screen.getByText('https://web-br-staging.example.com')).toBeTruthy()
    expect(screen.getByText(/fix login/)).toBeTruthy()
    const open = screen.getByLabelText('Open the staging deploy')
    expect(open.getAttribute('href')).toBe('https://web-br-staging.example.com')
  })

  it('shows the empty state with guidance when nothing is deployed', async () => {
    current = fixture({
      settings: { ...fixture().settings, mode: 'off', patterns: [] },
      deploys: [],
    })
    renderCard()
    expect(await screen.findByText('No branch deploys yet')).toBeTruthy()
    expect(screen.getByText(/Turn on branch deploys above/)).toBeTruthy()
  })

  it('saves the mode and patterns', async () => {
    const user = userEvent.setup()
    renderCard()
    const box = await screen.findByLabelText('Branches and patterns')
    await user.clear(box)
    await user.type(box, 'feature/*')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      const put = fetchMock.mock.calls.find(
        ([, init]) => (init as RequestInit | undefined)?.method === 'PUT',
      )
      expect(put).toBeTruthy()
      expect(JSON.parse(bodyOf(put?.[1] as RequestInit | undefined))).toEqual({
        mode: 'patterns',
        patterns: ['feature/*'],
      })
    })
  })

  it('deletes a deploy after confirmation', async () => {
    const user = userEvent.setup()
    renderCard()
    await user.click(
      await screen.findByRole('button', { name: 'Delete the staging deploy' }),
    )
    await user.click(
      await screen.findByRole('button', { name: 'Delete deploy' }),
    )

    await waitFor(() => {
      const del = fetchMock.mock.calls.find(
        ([, init]) => (init as RequestInit | undefined)?.method === 'DELETE',
      )
      expect(String(del?.[0])).toBe('/api/v1/apps/web/branch-deploys/staging')
    })
  })
})
