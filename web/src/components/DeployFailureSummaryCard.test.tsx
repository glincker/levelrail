import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import commonEn from '../locales/en/common.json'
import deployDetailEn from '../locales/en/deployDetail.json'
import type { DeployAttempt } from '../types/deployAttempt'
import type { DeployStage } from '../lib/deployStages'
import { DeployFailureSummaryCard } from './DeployFailureSummaryCard'

// No <RouterProvider> is mounted, so Link is stubbed to a plain anchor.
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({
      children,
      to,
      ...rest
    }: {
      children?: ReactNode
      to?: string
    } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
      <a href={to} {...rest}>
        {children}
      </a>
    ),
  }
})
vi.mock('../queries/gitSources', () => ({
  useGitSource: () => ({ data: { build_type: 'dockerfile' } }),
}))
let detailFailure: unknown
vi.mock('../queries/deployDetail', () => ({
  useDeployDetail: () => ({ data: { failure: detailFailure } }),
}))
vi.mock('../queries/rebuildApp', () => ({
  useRebuildApp: () => ({ isPending: false, mutate: vi.fn() }),
}))

// The card uses deployDetail strings and the retry menu uses common ones.
const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'deployDetail'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, deployDetail: deployDetailEn } },
  interpolation: { escapeValue: false },
})

const redeploy = vi.fn()
vi.mock('../hooks/useRedeployApp', () => ({
  useRedeployApp: () => ({ isPending: false, redeploy }),
}))
const mutate = vi.fn()
vi.mock('../queries/deploys', () => ({
  useTriggerDeploy: () => ({ isPending: false, mutate }),
}))

const failed: DeployAttempt = {
  id: 'f',
  service_name: 'web',
  image: 'web:2',
  status: 'failed',
  started_at: '2026-09-24T10:00:00Z',
  error: `failed to read dockerfile: open Dockerfile: no such file ${'x'.repeat(300)}`,
}
const good: DeployAttempt = {
  ...failed,
  id: 'g',
  image: 'web:1',
  status: 'succeeded',
  started_at: '2026-09-24T09:00:00Z',
  error: undefined,
}
const stages: DeployStage[] = [
  { key: 'build', label: 'Build', status: 'failed', detail: failed.error },
  { key: 'rollout', label: 'Roll out', status: 'skipped' },
]

function renderCard(attempts: DeployAttempt[]) {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <I18nextProvider i18n={testI18n}>
        <DeployFailureSummaryCard
          appName="web"
          attempt={failed}
          attempts={attempts}
          stages={stages}
          conditions={[]}
          lines={[]}
        />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('DeployFailureSummaryCard', () => {
  it('shows stage, cause, expandable error and actions', () => {
    renderCard([failed, good])
    expect(
      screen.getByRole('heading', { name: 'What went wrong' }),
    ).toBeTruthy()
    expect(
      screen.getByText('Dockerfile or build context not found'),
    ).toBeTruthy()
    const toggle = screen.getByRole('button', { name: 'Show more' })
    expect(toggle.getAttribute('aria-expanded')).toBe('false')
    fireEvent.click(toggle)
    expect(screen.getByRole('button', { name: 'Show less' })).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: /Retry deploy/ }))
    expect(redeploy).toHaveBeenCalled()
    fireEvent.click(
      screen.getByRole('button', { name: /Roll back to last good/ }),
    )
    expect(mutate).toHaveBeenCalledWith({ image: 'web:1' }, expect.any(Object))
  })

  it('hides rollback when there is no last good attempt', () => {
    renderCard([failed])
    expect(screen.queryByRole('button', { name: /Roll back/ })).toBeNull()
  })

  it('shows the control plane cause, docs link and typed fix link', () => {
    detailFailure = {
      code: 'missing_secret',
      cause: 'A required secret is not set',
      suggested_fix: 'Add it under environment variables.',
      docs_url: 'https://example.com/docs/secrets',
      retryable: false,
      fixes: [{ code: 's', kind: 'set_secret', hint: 'h', match: 'SECRET' }],
      deploy_id: 'f',
      app: 'web',
      at: '2026-09-24T10:01:00Z',
    }
    renderCard([failed, good])
    expect(screen.getByText('A required secret is not set')).toBeTruthy()
    expect(
      screen.getByText('Read the docs for this error').getAttribute('href'),
    ).toBe('https://example.com/docs/secrets')
    expect(screen.getByText('Edit environment variables')).toBeTruthy()
    detailFailure = undefined
  })
})
