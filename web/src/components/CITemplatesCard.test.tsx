import { Suspense } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import deploysEn from '../locales/en/deploys.json'
import { CITemplatesCard } from './CITemplatesCard'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['deploys'],
  defaultNS: 'deploys',
  resources: { en: { deploys: deploysEn } },
  interpolation: { escapeValue: false },
})

function jsonResponse(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

const templates = {
  templates: [
    {
      id: 'github',
      target: '.github/workflows/deploy.yml',
      auth: 'oidc',
      content: 'name: deploy-web',
    },
    {
      id: 'jenkins',
      target: 'Jenkinsfile',
      auth: 'token',
      content: 'pipeline { web }',
    },
  ],
  api_url: 'https://cp.example.test',
  needs_domain: false,
  deployer_abilities: ['read', 'deploy'],
  deployer_scope: {
    team_id: 'team_1',
    project_id: 'proj_1',
    team_role: 'developer',
  },
}

describe('CITemplatesCard', () => {
  const fetchMock = vi.fn()
  let payload: typeof templates | Record<string, unknown> = templates
  beforeEach(() => {
    payload = templates
    vi.stubGlobal('fetch', fetchMock)
    fetchMock.mockImplementation(
      (_input: RequestInfo | URL, init?: RequestInit) =>
        Promise.resolve(
          init?.method === 'POST'
            ? jsonResponse({ token: 'tok_once' })
            : jsonResponse(payload),
        ),
    )
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    fetchMock.mockReset()
  })

  it('shows the file per provider and reveals a deployer token once', async () => {
    const user = userEvent.setup()
    render(
      <I18nextProvider i18n={testI18n}>
        <QueryClientProvider client={createTestQueryClient()}>
          <Suspense fallback={null}>
            <CITemplatesCard appName="web" />
          </Suspense>
        </QueryClientProvider>
      </I18nextProvider>,
    )
    expect(await screen.findByText(/deploy-web/)).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'Create deployer token' }),
    ).toBeNull()

    await user.click(screen.getByRole('button', { name: 'Jenkins' }))
    await user.click(
      await screen.findByRole('button', { name: 'Create deployer token' }),
    )
    await waitFor(() =>
      expect(screen.getByLabelText('Deployer token')).toHaveValue('tok_once'),
    )
    const calls = fetchMock.mock.calls as [unknown, RequestInit | undefined][]
    const post = calls.find(([, init]) => init?.method === 'POST')
    expect(JSON.parse(post?.[1]?.body as string)).toEqual({
      name: 'ci-web',
      abilities: ['read', 'deploy'],
      team_id: 'team_1',
      project_id: 'proj_1',
      team_role: 'developer',
    })
    expect(
      screen.getByRole('button', { name: 'Create deployer token' }),
    ).toBeDisabled()
    expect(
      screen.getByText(
        'Set LEVELRAIL_API_URL to https://cp.example.test in your CI.',
      ),
    ).toBeVisible()
  })

  it('asks for an install-wide token without scope fields when the app is in no project', async () => {
    payload = { ...templates, deployer_scope: undefined }
    const user = userEvent.setup()
    render(
      <I18nextProvider i18n={testI18n}>
        <QueryClientProvider client={createTestQueryClient()}>
          <Suspense fallback={null}>
            <CITemplatesCard appName="web" />
          </Suspense>
        </QueryClientProvider>
      </I18nextProvider>,
    )
    await user.click(await screen.findByRole('button', { name: 'Jenkins' }))
    expect(
      screen.getByText(/in no project, so the token can deploy any app/),
    ).toBeVisible()
    await user.click(
      screen.getByRole('button', { name: 'Create deployer token' }),
    )
    await waitFor(() =>
      expect(screen.getByLabelText('Deployer token')).toBeVisible(),
    )
    const calls = fetchMock.mock.calls as [unknown, RequestInit | undefined][]
    const post = calls.find(([, init]) => init?.method === 'POST')
    expect(JSON.parse(post?.[1]?.body as string)).toEqual({
      name: 'ci-deployer',
      abilities: ['read', 'deploy'],
    })
  })
})
