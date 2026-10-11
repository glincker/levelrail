import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import gitBuildEn from '../locales/en/gitBuild.json'
import { GitBuildSettings } from './GitBuildSettings'
import type { GitBuildDetection, GitSourceResource } from '../types/gitSource'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['gitBuild'],
  defaultNS: 'gitBuild',
  resources: { en: { gitBuild: gitBuildEn } },
  interpolation: { escapeValue: false },
})

function source(over: Partial<GitSourceResource> = {}): GitSourceResource {
  return {
    service_name: 'glinr',
    repo_url: 'https://github.com/acme/mono.git',
    branch: 'main',
    build_type: 'railpack',
    trigger_mode: 'push',
    has_token: false,
    webhook_url: '/api/v1/webhooks/github/glinr',
    preview_enabled: false,
    post_pr_comments: false,
    deploy_paths: [],
    deploy_paths_ignore: [],
    report_status: true,
    created_at: '2026-10-10T00:00:00Z',
    updated_at: '2026-10-10T00:00:00Z',
    ...over,
  }
}

const turborepoDetection: GitBuildDetection = {
  branch: 'main',
  looks_like_monorepo: true,
  root_has_app: false,
  tools: ['turbo'],
  dockerfiles: ['apps/glinr/deploy/Dockerfile'],
  compose_files: [],
  truncated: false,
  needs_build_settings: true,
  suggestions: [
    {
      build_type: 'dockerfile',
      dockerfile_path: 'apps/glinr/deploy/Dockerfile',
      reason:
        'Dockerfile at apps/glinr/deploy/Dockerfile; it runs turbo prune.',
      reason_code: 'turbo_prune',
      recommended: true,
    },
  ],
}

interface FetchCall {
  url: string
  method: string
  body?: string
}

describe('GitBuildSettings', () => {
  let calls: FetchCall[]
  let detection: GitBuildDetection

  beforeEach(() => {
    calls = []
    detection = turborepoDetection
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url =
          typeof input === 'string'
            ? input
            : input instanceof URL
              ? input.toString()
              : input.url
        const method = init?.method ?? 'GET'
        calls.push({ url, method, body: init?.body as string | undefined })
        if (url.endsWith('/git-source/detect')) {
          return Promise.resolve(Response.json(detection))
        }
        if (url.endsWith('/git-source/build')) {
          const sent = JSON.parse(init?.body as string) as {
            build_type: GitSourceResource['build_type']
            build_path: string
            base_directory: string
          }
          return Promise.resolve(
            Response.json(
              source({
                build_type: sent.build_type,
                build_path: sent.build_path,
                base_directory: sent.base_directory,
              }),
            ),
          )
        }
        return Promise.resolve(new Response('not found', { status: 404 }))
      }),
    )
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  function renderIt(src: GitSourceResource) {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    return render(
      <I18nextProvider i18n={testI18n}>
        <QueryClientProvider client={client}>
          <GitBuildSettings appName="glinr" source={src} />
        </QueryClientProvider>
      </I18nextProvider>,
    )
  }

  it('nudges a monorepo on auto-detect and applies a suggestion in one click', async () => {
    const user = userEvent.setup()
    renderIt(source())

    await screen.findByText('This looks like a monorepo. Pick a Dockerfile.')
    await user.click(screen.getByRole('button', { name: 'Pick a Dockerfile' }))

    await screen.findByText('Dockerfile apps/glinr/deploy/Dockerfile')
    expect(
      screen.getByText(/It runs turbo prune, so keep the build context/),
    ).toBeInTheDocument()
    expect(screen.getByText('Recommended')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Use this' }))

    await waitFor(() => {
      expect(
        calls.some(
          (c) => c.url.endsWith('/git-source/build') && c.method === 'PUT',
        ),
      ).toBe(true)
    })
    const put = calls.find((c) => c.url.endsWith('/git-source/build'))
    expect(JSON.parse(put?.body ?? '{}')).toEqual({
      build_type: 'dockerfile',
      build_path: 'apps/glinr/deploy/Dockerfile',
      base_directory: '',
    })
  })

  it('does not nudge a single app at the repository root', async () => {
    detection = {
      ...turborepoDetection,
      looks_like_monorepo: false,
      root_has_app: true,
      needs_build_settings: false,
      suggestions: [],
      dockerfiles: [],
    }
    renderIt(source())

    await waitFor(() => {
      expect(calls.some((c) => c.url.endsWith('/git-source/detect'))).toBe(true)
    })
    expect(
      screen.queryByText('This looks like a monorepo. Pick a Dockerfile.'),
    ).not.toBeInTheDocument()
  })

  it('states the Dockerfile, base directory and resolved context in plain words', () => {
    renderIt(
      source({
        build_type: 'dockerfile',
        build_path: 'apps/api/Dockerfile.prod',
        base_directory: 'apps/api',
      }),
    )

    expect(screen.getByText('apps/api/Dockerfile.prod')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Builds apps/api/Dockerfile.prod with apps/api as the build context.',
      ),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Change build settings' }),
    ).toBeInTheDocument()
    expect(calls).toHaveLength(0)
  })

  it('blocks saving a Dockerfile outside the base directory', async () => {
    const user = userEvent.setup()
    renderIt(source({ build_type: 'dockerfile', base_directory: 'apps/api' }))

    await user.click(
      screen.getByRole('button', { name: 'Change build settings' }),
    )
    await user.type(screen.getByLabelText('Dockerfile path'), 'Dockerfile')

    expect(
      await screen.findByText(
        /The Dockerfile must be inside the base directory/,
      ),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Save build settings' }),
    ).toBeDisabled()
  })
})
