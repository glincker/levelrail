import type { ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { GitSourceSwitchPlan } from './GitSourceSwitchPlan'
import { GitSourceDeployNow } from './GitSourceDeployNow'
import { AllI18n } from '../test/allI18n'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children?: ReactNode }) => <a>{children}</a>,
}))

function wrap(ui: ReactNode) {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <AllI18n>{ui}</AllI18n>
    </QueryClientProvider>,
  )
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('GitSourceSwitchPlan', () => {
  it('shows what an upload site keeps and applies the static suggestion', async () => {
    const fetchMock = vi.fn(() =>
      Promise.resolve(
        jsonResponse({
          app: 'site',
          current_source: 'upload',
          static: true,
          branch: 'main',
          preserved: {
            env_vars: 2,
            secret_env: 0,
            domains: ['site.example.test'],
            volumes: [],
            replicas: 1,
            running_image: 'site:abc',
          },
          suggestion: {
            build_type: 'static',
            build_command: 'npm run build',
            publish_dir: 'dist',
          },
          warnings: ['uploads_stop'],
        }),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    const onApply = vi.fn()
    wrap(
      <GitSourceSwitchPlan
        appName="site"
        repoUrl="https://github.com/acme/site"
        branch="main"
        onApply={onApply}
      />,
    )
    expect(
      await screen.findByText('This app is served from uploaded files today.'),
    ).toBeInTheDocument()
    expect(screen.getByText('2 env vars kept')).toBeInTheDocument()
    await userEvent.click(
      screen.getByRole('button', { name: 'Use these settings' }),
    )
    expect(onApply).toHaveBeenCalledWith(
      expect.objectContaining({ build_type: 'static', publish_dir: 'dist' }),
    )
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/apps/site/git-source/plan',
      expect.objectContaining({ method: 'POST' }),
    )
  })
})

describe('GitSourceDeployNow', () => {
  it('starts a build of the connected branch', async () => {
    const fetchMock = vi.fn(() =>
      Promise.resolve(
        jsonResponse({ app: 'web', branch: 'main', started: true }, 202),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    wrap(<GitSourceDeployNow appName="web" branch="main" />)
    await userEvent.click(screen.getByRole('button', { name: 'Deploy now' }))
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/apps/web/git-source/deploy',
        expect.objectContaining({ method: 'POST' }),
      )
    })
    expect(await screen.findByText('View deploy')).toBeInTheDocument()
  })
})
