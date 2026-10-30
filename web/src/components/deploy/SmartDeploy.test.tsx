import { Suspense } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import smartDeployEn from '../../locales/en/smartDeploy.json'
import commonEn from '../../locales/en/common.json'
import { DeploySheet } from '../overview/DeploySheet'
import { createTestQueryClient } from '@/test/testQueryClient'

const navigate = vi.fn()
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigate,
}))
vi.mock('@/components/ui/toast', () => ({
  toast: { add: vi.fn() },
  toastAction: vi.fn(),
}))
vi.mock('../GitRepoSourcePicker', () => ({
  GitRepoSourcePicker: ({
    onSelect,
  }: {
    onSelect: (v: { provider: string; repoUrl: string; branch: string }) => void
  }) => (
    <button
      type="button"
      onClick={() => {
        onSelect({
          provider: 'manual',
          repoUrl: 'https://github.com/acme/web.git',
          branch: 'main',
        })
      }}
    >
      pick repo
    </button>
  ),
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['smartDeploy', 'common'],
  defaultNS: 'smartDeploy',
  resources: { en: { smartDeploy: smartDeployEn, common: commonEn } },
  interpolation: { escapeValue: false },
})

const baseApp = { name: 'web', image: 'registry/web:abc1234', port: 80 }

const gitSource = {
  service_name: 'web',
  repo_url: 'https://github.com/acme/web.git',
  branch: 'main',
  build_type: 'dockerfile',
  build_path: 'deploy/Dockerfile',
  trigger_mode: 'push',
}

const commit = (sha: string, message: string, status?: string) => ({
  sha,
  message,
  author: 'Ann',
  authored_at: new Date().toISOString(),
  ...(status ? { deploy_status: status, deploy_id: 'dep_' + sha } : {}),
})

function commitsBody(over: Record<string, unknown> = {}) {
  return {
    repo_url: gitSource.repo_url,
    branch: 'main',
    connected_branch: 'main',
    provider: 'github',
    head: commit('c3c3c3c3c3', 'feat: new thing'),
    commits: [
      commit('c3c3c3c3c3', 'feat: new thing'),
      commit('b2b2b2b2b2', 'fix: older', 'deployed'),
    ],
    branches: ['main', 'dev'],
    ahead_by: 1,
    up_to_date: false,
    deployed: {
      deploy_id: 'dep_b',
      image: 'web:b2b2',
      commit_sha: 'b2b2b2b2b2',
    },
    last_successful: {
      deploy_id: 'dep_b',
      image: 'registry/web:b2b2',
      finished_at: new Date().toISOString(),
    },
    previous_successful: {
      deploy_id: 'dep_a',
      image: 'registry/web:a1a1',
      finished_at: new Date().toISOString(),
    },
    build: {
      context_dir: '.',
      dockerfile_path: 'deploy/Dockerfile',
      summary: '',
    },
    build_type: 'dockerfile',
    trigger_mode: 'push',
    suggestions: [
      {
        id: 'add_health_check',
        severity: 'info',
        reason: 'No health check',
        action: { type: 'open_health_settings' },
      },
    ],
    degraded: false,
    fetched_at: new Date().toISOString(),
    ...over,
  }
}

type Handler = (init?: RequestInit) => Response
let routes: Record<string, Handler>
const calls: { url: string; method: string; body?: string }[] = []

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

beforeEach(() => {
  calls.length = 0
  navigate.mockReset()
  routes = {}
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url =
        typeof input === 'string'
          ? input
          : input instanceof URL
            ? input.href
            : input.url
      const path = url.split('?')[0] ?? url
      calls.push({
        url,
        method: init?.method ?? 'GET',
        body: typeof init?.body === 'string' ? init.body : undefined,
      })
      const key = `${init?.method ?? 'GET'} ${path}`
      const handler = routes[key]
      return Promise.resolve(handler ? handler(init) : json({}, 404))
    }),
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
})

function renderSheet(
  tab: 'existing-image' | 'build-from-source' = 'build-from-source',
) {
  const client = createTestQueryClient()
  const onClose = vi.fn()
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={client}>
        <Suspense fallback={<div>loading</div>}>
          <DeploySheet appName="web" tab={tab} onClose={onClose} />
        </Suspense>
      </QueryClientProvider>
    </I18nextProvider>,
  )
  return { onClose }
}

describe('smart deploy sheet', () => {
  it('git connected and ahead: shows head, status, suggestion and deploys it', async () => {
    routes['GET /api/v1/apps/web'] = () => json(baseApp)
    routes['GET /api/v1/apps/web/git-source'] = () => json(gitSource)
    routes['GET /api/v1/apps/web/git-source/commits'] = () =>
      json(commitsBody())
    routes['POST /api/v1/apps/web/builds'] = () => json({ id: 'dep_new' }, 202)

    const { onClose } = renderSheet('existing-image')

    expect(
      await screen.findByText('Deploy latest commit on main'),
    ).toBeInTheDocument()
    expect(screen.getByText('feat: new thing')).toBeInTheDocument()
    expect(
      screen.getByText('1 commit ahead of what is live'),
    ).toBeInTheDocument()
    expect(screen.getByText('Add a health check')).toBeInTheDocument()
    expect(screen.queryByText('Existing image')).not.toBeInTheDocument()

    await userEvent.click(
      screen.getByRole('button', { name: 'Deploy latest commit' }),
    )

    await waitFor(() => {
      expect(
        calls.some((c) => c.method === 'POST' && c.url.endsWith('/builds')),
      ).toBe(true)
    })
    const post = calls.find((c) => c.method === 'POST')
    const body = JSON.parse(post?.body ?? '{}') as Record<string, unknown>
    expect(body.ref).toBe('c3c3c3c3c3')
    expect(body.repo_url).toBe(gitSource.repo_url)
    expect(body.build).toMatchObject({
      type: 'dockerfile',
      path: 'deploy/Dockerfile',
    })
    await waitFor(() => {
      expect(navigate).toHaveBeenCalledWith({
        to: '/apps/$name/deploys/$deployId/logs',
        params: { name: 'web', deployId: 'dep_new' },
      })
    })
    expect(onClose).toHaveBeenCalled()
  })

  it('up to date: says so and offers shortcuts', async () => {
    routes['GET /api/v1/apps/web'] = () => json(baseApp)
    routes['GET /api/v1/apps/web/git-source'] = () => json(gitSource)
    routes['GET /api/v1/apps/web/git-source/commits'] = () =>
      json(commitsBody({ up_to_date: true, ahead_by: 0 }))
    renderSheet()

    expect(
      await screen.findByText('Up to date, c3c3c3c is live'),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /Redeploy last successful/ }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /Roll back to previous/ }),
    ).toBeInTheDocument()
  })

  it('dismissing a suggestion is remembered', async () => {
    routes['GET /api/v1/apps/web'] = () => json(baseApp)
    routes['GET /api/v1/apps/web/git-source'] = () => json(gitSource)
    routes['GET /api/v1/apps/web/git-source/commits'] = () =>
      json(commitsBody())
    renderSheet()

    await screen.findByText('Add a health check')
    await userEvent.click(screen.getByRole('button', { name: 'Dismiss' }))
    expect(screen.queryByText('Add a health check')).not.toBeInTheDocument()
    expect(
      window.localStorage.getItem(
        'levelrail.deploySuggestions.dismissed.v1.web',
      ),
    ).toContain('add_health_check')
  })

  it('provider failure shows the message and a fallback deploy of the branch head', async () => {
    routes['GET /api/v1/apps/web'] = () => json(baseApp)
    routes['GET /api/v1/apps/web/git-source'] = () => json(gitSource)
    routes['GET /api/v1/apps/web/git-source/commits'] = () =>
      json({ error: 'the provider refused access to this repository' }, 422)
    renderSheet()

    expect(await screen.findByText(/refused access/)).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Deploy head of main' }),
    ).toBeInTheDocument()
  })

  it('image only: pre-filled image, advanced hides the fresh lookup', async () => {
    routes['GET /api/v1/apps/web'] = () => json(baseApp)
    routes['GET /api/v1/apps/web/images'] = () =>
      json([{ tag: 'registry/web:old1', created_at: '' }])
    routes['POST /api/v1/apps/web/deploys'] = () => json({ ...baseApp })
    renderSheet('existing-image')

    const input = await screen.findByLabelText('Image')
    expect(input).toHaveValue('registry/web:abc1234')
    expect(
      await screen.findByRole('button', { name: 'old1' }),
    ).toBeInTheDocument()
    expect(screen.queryByText(/fresh registry lookup/i)).not.toBeInTheDocument()
    expect(
      screen.getByRole('button', {
        name: 'Deploy from a git repository instead',
      }),
    ).toBeInTheDocument()
  })

  it('no source at all: guided connect flow', async () => {
    routes['GET /api/v1/apps/web'] = () => json({ ...baseApp, image: '' })
    renderSheet('existing-image')

    expect(await screen.findByText('Connect a repository')).toBeInTheDocument()
    expect(screen.getByText('Or deploy a container image')).toBeInTheDocument()
  })

  it('protected environment requires acknowledgement before deploying', async () => {
    routes['GET /api/v1/apps/web'] = () =>
      json({ ...baseApp, project_id: 'p1', environment_id: 'e1' })
    routes['GET /api/v1/projects/p1/environments'] = () =>
      json([{ id: 'e1', name: 'production', protected: true }])
    routes['GET /api/v1/apps/web/git-source'] = () => json(gitSource)
    routes['GET /api/v1/apps/web/git-source/commits'] = () =>
      json(commitsBody())
    renderSheet()

    const deploy = await screen.findByRole('button', {
      name: 'Deploy latest commit',
    })
    expect(await screen.findByText(/protected environment/)).toBeInTheDocument()
    expect(deploy).toBeDisabled()
    await userEvent.click(screen.getByRole('checkbox'))
    expect(deploy).toBeEnabled()
  })
})
