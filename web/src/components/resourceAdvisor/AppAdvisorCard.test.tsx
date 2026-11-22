import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import appAdvisor from '../../locales/en/appAdvisor.json'
import resourceAdvisor from '../../locales/en/resourceAdvisor.json'
import { AppAdvisorCard } from './AppAdvisorCard'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))
vi.mock('../../hooks/useExperimental', () => ({
  useExperimentalFeatures: () => ['resource-advisor'],
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['appAdvisor', 'resourceAdvisor'],
  defaultNS: 'appAdvisor',
  resources: { en: { appAdvisor, resourceAdvisor } },
  interpolation: { escapeValue: false },
})

const finding = {
  id: 'memory_limit_missing:app:web',
  kind: 'memory_limit_missing',
  severity: 'suggest',
  resource: { kind: 'app', name: 'web' },
  title: 'Set a memory limit on web',
  why: '',
  evidence: {},
  impact: {},
  action: { type: 'set_memory_limit', value: 1 },
  appliable: true,
  can_apply: true,
}

afterEach(() => vi.unstubAllGlobals())

describe('AppAdvisorCard', () => {
  it('lists the app findings and opens the apply preview', async () => {
    const fetchFn = vi.fn((input: string) =>
      Promise.resolve({
        ok: true,
        status: 200,
        json: () =>
          Promise.resolve(
            input.includes('/apply')
              ? {
                  finding_id: finding.id,
                  resource: finding.resource,
                  action: finding.action,
                  before: { memory_bytes: 0, nano_cpus: 0 },
                  after: { memory_bytes: 1048576, nano_cpus: 0 },
                  confirm_required: true,
                  applied: false,
                }
              : { generated_at: '', findings: [finding] },
          ),
      } as unknown as Response),
    )
    vi.stubGlobal('fetch', fetchFn)
    render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={createTestQueryClient()}>
          <AppAdvisorCard name="web" />
        </QueryClientProvider>
      </I18nextProvider>,
    )
    await userEvent.click(
      await screen.findByRole('button', {
        name: 'Apply: Set a memory limit on web',
      }),
    )
    await waitFor(() => {
      expect(
        fetchFn.mock.calls.some(([u]) => String(u).includes('/apply')),
      ).toBe(true)
    })
    expect(fetchFn.mock.calls[0]?.[0]).toContain('resource=app%3Aweb')
  })
})
