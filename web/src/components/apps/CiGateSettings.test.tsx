import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { CiGateSettings } from './CiGateSettings'
import gitBuildEn from '../../locales/en/gitBuild.json'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['gitBuild'],
  defaultNS: 'gitBuild',
  resources: { en: { gitBuild: gitBuildEn } },
  interpolation: { escapeValue: false },
})

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

describe('CiGateSettings', () => {
  const fetchMock = vi.fn()
  beforeEach(() => {
    vi.stubGlobal('fetch', fetchMock)
    fetchMock.mockImplementation(
      (_input: RequestInfo | URL, init?: RequestInit) =>
        Promise.resolve(
          init?.method === 'PUT'
            ? json(
                {
                  error: 'wait_for_ci needs a GitHub App or GitLab connection',
                },
                422,
              )
            : json({
                repo_url: 'git@example.test:a/b.git',
                wait_for_ci: false,
                ci_required_checks: [],
              }),
        ),
    )
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    fetchMock.mockReset()
  })

  function renderCard() {
    return render(
      <I18nextProvider i18n={testI18n}>
        <QueryClientProvider client={createTestQueryClient()}>
          <CiGateSettings appName="web" />
        </QueryClientProvider>
      </I18nextProvider>,
    )
  }

  it('says GitHub and GitLab are supported and explains the required check names', async () => {
    const user = userEvent.setup()
    renderCard()
    expect(
      await screen.findByText(/Works with GitHub and GitLab/),
    ).toBeVisible()
    await user.click(screen.getByRole('switch', { name: 'Wait for CI checks' }))
    expect(screen.getByText(/On GitLab use the job name/)).toBeVisible()
    expect(screen.getByText(/allow_failure/)).toBeVisible()
  })

  it('explains an unreadable source when the API answers 422', async () => {
    const user = userEvent.setup()
    renderCard()
    await user.click(
      await screen.findByRole('switch', { name: 'Wait for CI checks' }),
    )
    await user.click(screen.getByRole('button', { name: 'Save' }))
    const alert = await screen.findByRole('alert')
    await waitFor(() =>
      expect(alert).toHaveTextContent(
        "This repository's CI status cannot be read",
      ),
    )
    for (const reason of [
      'SSH or plain http',
      'Gitea',
      'Bitbucket',
      'without the GitHub App installed',
    ]) {
      expect(alert).toHaveTextContent(reason)
    }
  })
})
