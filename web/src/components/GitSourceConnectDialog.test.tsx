import type { ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { GitSourceConnectDialog } from './GitSourceConnectDialog'
import type { GitRepoSourceValue } from './GitRepoSourcePicker'
import { AllI18n } from '../test/allI18n'
import { createTestQueryClient } from '@/test/testQueryClient'

const connectPickedRepo =
  vi.fn<(...args: unknown[]) => Promise<{ branch: string }>>()

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children?: ReactNode }) => <a>{children}</a>,
}))
vi.mock('./deploy/connectSource', () => ({
  connectPickedRepo: (...args: unknown[]) => connectPickedRepo(...args),
}))
vi.mock('./GitRepoSourcePicker', () => ({
  GitRepoSourcePicker: ({
    onSelect,
  }: {
    onSelect: (v: GitRepoSourceValue) => void
  }) => (
    <button
      type="button"
      onClick={() =>
        onSelect({
          provider: 'gitlab',
          repoUrl: 'https://gitlab.example.test/g/web',
          branch: 'main',
          providerRef: { kind: 'gitlab', connectionId: 'gc_1', repo: '7' },
        })
      }
    >
      pick-repo
    </button>
  ),
}))

function plan(warnings: string[] = []) {
  return {
    app: 'api',
    current_source: 'image',
    static: false,
    branch: 'main',
    preserved: {
      env_vars: 1,
      secret_env: 1,
      domains: [],
      volumes: [],
      replicas: 1,
    },
    suggestion: { build_type: 'railpack', base_directory: 'services/a' },
    warnings,
    group: [{ name: 'worker', current_source: 'image', effect: 'unchanged' }],
  }
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

function stubFetch(warnings: string[] = []) {
  const calls: { url: string; body: string }[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      calls.push({ url, body: typeof init?.body === 'string' ? init.body : '' })
      if (url.endsWith('/git-source/plan'))
        return Promise.resolve(jsonResponse(plan(warnings)))
      return Promise.resolve(
        jsonResponse({ app: 'worker', branch: 'main', started: true }, 202),
      )
    }),
  )
  return calls
}

function wrap(onOpenChange = vi.fn()) {
  render(
    <QueryClientProvider client={createTestQueryClient()}>
      <AllI18n>
        <GitSourceConnectDialog
          appName="api"
          open
          onOpenChange={onOpenChange}
        />
      </AllI18n>
    </QueryClientProvider>,
  )
  return onOpenChange
}

async function toReview() {
  const user = userEvent.setup()
  await user.click(screen.getByText('pick-repo'))
  await user.click(screen.getByRole('button', { name: 'Next' }))
  await screen.findByText('Which service does this repository build?')
  return user
}

beforeEach(() => {
  connectPickedRepo.mockResolvedValue({ branch: 'main' })
})

afterEach(() => {
  vi.unstubAllGlobals()
  connectPickedRepo.mockReset()
})

describe('GitSourceConnectDialog', () => {
  it('plans with the picked connection and deploys the chosen service', async () => {
    const calls = stubFetch()
    wrap()
    const user = await toReview()
    expect(calls[0]?.body).toContain('"connection_id":"gc_1"')
    await user.click(screen.getByRole('radio', { name: /worker/ }))
    await waitFor(() =>
      expect(
        calls.some((c) => c.url.includes('/apps/worker/git-source/plan')),
      ).toBe(true),
    )
    await screen.findByRole('radio', { name: /worker/ })
    await waitFor(() =>
      expect(
        screen.getByRole<HTMLButtonElement>('button', { name: /Deploy now/ })
          .disabled,
      ).toBe(false),
    )
    await user.click(screen.getByRole('button', { name: /Deploy now/ }))
    await waitFor(() =>
      expect(connectPickedRepo).toHaveBeenCalledWith(
        'worker',
        expect.objectContaining({
          repoUrl: 'https://gitlab.example.test/g/web',
        }),
        expect.objectContaining({ base_directory: 'services/a' }),
      ),
    )
    await waitFor(() =>
      expect(
        calls.some((c) => c.url.includes('/apps/worker/git-source/deploy')),
      ).toBe(true),
    )
    expect(
      await screen.findByText(
        'Step 3 of 3: the first deploy from this repository.',
      ),
    ).toBeTruthy()
  })

  it('links only without starting a deploy', async () => {
    const calls = stubFetch()
    const onOpenChange = wrap()
    const user = await toReview()
    await user.click(screen.getByRole('button', { name: 'Link only' }))
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(connectPickedRepo).toHaveBeenCalledWith(
      'api',
      expect.anything(),
      expect.objectContaining({ build_type: 'railpack' }),
    )
    expect(calls.some((c) => c.url.includes('/git-source/deploy'))).toBe(false)
  })

  it('explains a deploy-only caller and blocks deploy when protection is unknown', async () => {
    stubFetch(['connection_needs_read_sensitive', 'protection_unknown'])
    wrap()
    await toReview()
    expect(screen.getByText(/cannot use the picked connection/)).toBeTruthy()
    expect(
      screen.getByRole<HTMLButtonElement>('button', { name: /Deploy now/ })
        .disabled,
    ).toBe(true)
  })
})
