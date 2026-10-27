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
vi.mock('../../queries/appNetwork', () => ({
  useAppNetwork: () => ({ data: undefined }),
}))
const urlsState = vi.hoisted(() => ({
  data: undefined as { addresses: object[] } | undefined,
}))
const httpsState = vi.hoisted(() => ({
  data: undefined as { state: string } | undefined,
}))
vi.mock('../../queries/appUrls', () => ({
  useAppUrls: () => ({ data: urlsState.data }),
}))
vi.mock('../../hooks/useIsRoot', () => ({ useIsRoot: () => true }))
vi.mock('@tanstack/react-query', () => ({
  useQuery: () => ({ data: httpsState.data }),
}))
vi.mock('../../queries/httpsStatus', () => ({
  httpsStatusQueryOptions: () => ({}),
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

describe('FirstAppStatus address with HTTPS off', () => {
  it('opens the direct address and explains it is not secure yet', () => {
    httpsState.data = { state: 'off' }
    urlsState.data = {
      addresses: [
        {
          kind: 'public',
          url: 'http://203.0.113.5:8080',
          reachable_hint: 'reachable',
        },
      ],
    }
    renderPhase('live')
    expect(
      screen.getByText(/203\.0\.113\.5:8080/).closest('a'),
    ).toHaveAttribute('href', 'http://203.0.113.5:8080')
    expect(
      screen.getByText(getStartedEn.first.status.notSecure, { exact: false }),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'Enable HTTPS' }),
    ).not.toBeInTheDocument()
  })

  it('shows no address and no secure note when the API reports none', () => {
    httpsState.data = { state: 'off' }
    urlsState.data = { addresses: [] }
    renderPhase('live')
    expect(
      screen.getByText(getStartedEn.first.status.noAddress),
    ).toBeInTheDocument()
    expect(
      screen.queryByText(getStartedEn.first.status.notSecure, { exact: false }),
    ).not.toBeInTheDocument()
  })
})

describe('FirstAppStatus live actions', () => {
  it('shows the address pill without a second Open app button', () => {
    httpsState.data = { state: 'off' }
    urlsState.data = {
      addresses: [
        {
          kind: 'public',
          url: 'http://203.0.113.5:8080',
          reachable_hint: 'reachable',
        },
      ],
    }
    renderPhase('live')
    expect(
      screen.queryByRole('link', { name: getStartedEn.first.status.openApp }),
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: getStartedEn.first.status.viewLogs }),
    ).toBeInTheDocument()
  })
})

describe('FirstAppStatus live state', () => {
  it('shows one status line and keeps the go-live action reachable', () => {
    renderPhase('live')
    expect(screen.queryByText('It is live')).not.toBeInTheDocument()
    expect(
      screen.getByText(
        getStartedEn.first.status.liveDetail.replace('{{name}}', app.name),
      ),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: getStartedEn.first.status.goLive }),
    ).toHaveAttribute('href', '/apps/$name/go-live')
  })
})
