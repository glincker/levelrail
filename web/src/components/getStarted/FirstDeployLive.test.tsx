import type { ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import getStartedEn from '../../locales/en/getStarted.json'
import { FirstDeploy } from './FirstDeploy'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))
vi.mock('../CreateResourceWizard', () => ({
  CreateResourceWizard: ({ trigger }: { trigger: ReactNode }) => <>{trigger}</>,
}))
vi.mock('../EnableHttpsCard', () => ({ EnableHttpsCard: () => null }))
vi.mock('./FirstAppStatus', () => ({ FirstAppStatus: () => null }))
vi.mock('../../hooks/useIsRoot', () => ({ useIsRoot: () => true }))
vi.mock('./useFirstAppPolling', () => ({
  useFirstAppPolling: () => ({ app: { name: 'sample-app' }, phase: 'live' }),
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['getStarted'],
  defaultNS: 'getStarted',
  resources: { en: { getStarted: getStartedEn } },
  interpolation: { escapeValue: false },
})

describe('FirstDeploy once the app is live', () => {
  it('switches the heading to the live title and drops the pick-a-path subtitle', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={new QueryClient()}>
          <FirstDeploy onStarted={vi.fn()} onDone={vi.fn()} />
        </QueryClientProvider>
      </I18nextProvider>,
    )
    expect(
      screen.getByRole('heading', { name: getStartedEn.first.titleLive }),
    ).toBeTruthy()
    expect(screen.queryByText(getStartedEn.first.subtitle)).toBeNull()
  })
})
