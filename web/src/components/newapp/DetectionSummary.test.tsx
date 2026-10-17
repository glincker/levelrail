import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DetectionSummary } from './DetectionSummary'
import { MonorepoPackages } from './MonorepoPackages'
import { NewAppI18n } from '../../test/newAppI18n'
import { jsonResponse, nextAnalysis } from '../../test/newAppFixtures'
import {
  clearImportDraft,
  loadImportDraft,
  saveImportDraft,
} from '../../lib/importDraft'
import type { Detection } from '../../queries/importAnalyze'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
      <a href={to}>{children}</a>
    ),
  }
})

const detection: Detection = {
  framework: 'Next.js',
  language: 'node',
  build_type: 'railpack',
  confidence: 'high',
  reasons: [
    { signal: 'package.json', detail: 'package.json has next in dependencies' },
  ],
  port: { value: 3000, source: 'Next.js default port' },
  start_command: 'next start',
  health_path: '/',
  runtime_version: { value: '20', source: '.nvmrc' },
  env_vars: [],
  warnings: [{ code: 'no_lockfile', message: 'no lockfile' }],
}

afterEach(() => {
  vi.restoreAllMocks()
  clearImportDraft()
})

describe('DetectionSummary', () => {
  it('shows confidence, fields and the reasons behind the framework', async () => {
    const user = userEvent.setup()
    render(
      <NewAppI18n>
        <DetectionSummary detection={detection} />
      </NewAppI18n>,
    )
    expect(screen.getByText('High confidence')).toBeInTheDocument()
    expect(screen.getByText('next start')).toBeInTheDocument()
    expect(screen.getByText(/Commit your lockfile/)).toBeInTheDocument()
    await user.click(
      screen.getByRole('button', {
        name: 'Why Framework and language was detected',
      }),
    )
    expect(
      await screen.findByText('package.json has next in dependencies'),
    ).toBeInTheDocument()
  })
})

describe('MonorepoPackages', () => {
  it('creates one app per selected package with its watch paths', async () => {
    const user = userEvent.setup()
    const calls: string[] = []
    vi.spyOn(globalThis, 'fetch').mockImplementation((input, init) => {
      const url =
        typeof input === 'string'
          ? input
          : input instanceof URL
            ? input.href
            : input.url
      calls.push(`${init?.method ?? 'GET'} ${url}`)
      if (url.endsWith('/imports/analyze')) {
        return Promise.resolve(
          jsonResponse({
            ...nextAnalysis,
            name: { value: 'mono-web', evidence: '' },
            databases: [],
            env: [],
          }),
        )
      }
      if (url.endsWith('/builds')) {
        return Promise.resolve(jsonResponse({ id: 'att1' }))
      }
      return Promise.resolve(jsonResponse({}))
    })
    const client = new QueryClient()
    render(
      <QueryClientProvider client={client}>
        <NewAppI18n>
          <MonorepoPackages
            packages={[
              {
                name: 'web',
                dir: 'apps/web',
                tool: 'pnpm-workspace',
                depends_on: [],
                watch_paths: ['apps/web/**'],
              },
            ]}
            source={{
              provider: 'manual',
              repoUrl: 'https://github.com/acme/mono.git',
              branch: 'main',
            }}
            onPick={() => undefined}
          />
        </NewAppI18n>
      </QueryClientProvider>,
    )
    await user.click(screen.getByRole('checkbox'))
    await user.click(screen.getByRole('button', { name: /Create an app for/ }))
    await waitFor(() => {
      expect(calls).toContain(
        'PUT /api/v1/apps/mono-web/git-source/deploy-settings',
      )
    })
    expect(await screen.findByText('Open mono-web build')).toBeInTheDocument()
  })
})

describe('importDraft', () => {
  it('round-trips and rejects malformed storage', () => {
    saveImportDraft({
      step: 'findings',
      source: {
        provider: 'manual',
        repoUrl: 'https://x/y.git',
        branch: 'main',
      },
      baseDirectory: 'apps/web',
    })
    expect(loadImportDraft()?.baseDirectory).toBe('apps/web')
    sessionStorage.setItem('import-repo-draft', '{nope')
    expect(loadImportDraft()).toBeNull()
  })
})
