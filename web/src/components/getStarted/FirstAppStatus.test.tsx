import type { ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import getStartedEn from '../../locales/en/getStarted.json'
import type { AppListEntry } from '../../types/appDetail'
import { FirstAppStatus } from './FirstAppStatus'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))
vi.mock('../golive/GoLiveCta', () => ({ GoLiveCta: () => null }))
vi.mock('../../queries/appNetwork', () => ({
  useAppNetwork: () => ({ data: undefined }),
}))
vi.mock('../../queries/diagnosis', () => ({
  useDiagnosis: () => ({ data: undefined, isLoading: false }),
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['getStarted'],
  defaultNS: 'getStarted',
  resources: { en: { getStarted: getStartedEn } },
  interpolation: { escapeValue: false },
})

const app = {
  name: 'sample-app',
  status: { label: 'Starting' },
} as unknown as AppListEntry

function renderPhase(phase: 'deploying' | 'slow' | 'live') {
  return render(
    <I18nextProvider i18n={i18n}>
      <FirstAppStatus app={app} phase={phase} />
    </I18nextProvider>,
  )
}

describe('FirstAppStatus expected wait', () => {
  it('tells the user a first start can take up to two minutes while deploying', () => {
    renderPhase('deploying')
    expect(
      screen.getByText(getStartedEn.first.status.expectedWait),
    ).toBeInTheDocument()
  })

  it('does not show the wait line once the app is live', () => {
    renderPhase('live')
    expect(
      screen.queryByText(getStartedEn.first.status.expectedWait),
    ).not.toBeInTheDocument()
  })
})
