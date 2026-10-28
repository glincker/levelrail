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
const networkState = vi.hoisted(() => ({
  data: undefined as { fallback_url?: string } | undefined,
}))
const attemptsState = vi.hoisted(() => ({
  data: undefined as object[] | undefined,
}))
const redeploy = vi.hoisted(() => vi.fn())
vi.mock('../../queries/appNetwork', () => ({
  useAppNetwork: () => ({ data: networkState.data }),
}))
vi.mock('../../queries/deployAttempts', () => ({
  deployAttemptsQueryOptions: () => ({ queryKey: ['attempts'] }),
}))
vi.mock('../../queries/deploys', () => ({
  useTriggerDeploy: () => ({ mutate: redeploy, isPending: false }),
}))
const urlsState = vi.hoisted(() => ({
  data: undefined as
    { addresses: object[]; http_fallback_url?: string } | undefined,
}))
const httpsState = vi.hoisted(() => ({
  data: undefined as { state: string } | undefined,
}))
vi.mock('../../queries/appUrls', () => ({
  useAppUrls: () => ({ data: urlsState.data }),
}))
vi.mock('../../hooks/useIsRoot', () => ({ useIsRoot: () => true }))
vi.mock('@tanstack/react-query', () => ({
  useQuery: (o: { queryKey?: string[] }) => ({
    data: o.queryKey?.[0] === 'attempts' ? attemptsState.data : httpsState.data,
  }),
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

function renderPhase(
  phase: 'deploying' | 'slow' | 'live' | 'failed',
  target: AppListEntry = app,
) {
  return render(
    <I18nextProvider i18n={i18n}>
      <FirstAppStatus app={target} phase={phase} />
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
  it('opens the direct address and leaves the not-secure wording to the HTTPS card', () => {
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
    expect(screen.queryByText(/not secure/i)).not.toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'Enable HTTPS' }),
    ).not.toBeInTheDocument()
  })

  it('shows no address when the API reports none', () => {
    httpsState.data = { state: 'off' }
    urlsState.data = { addresses: [] }
    renderPhase('live')
    expect(
      screen.getByText(getStartedEn.first.status.noAddress),
    ).toBeInTheDocument()
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

describe('FirstAppStatus address priority', () => {
  const fallback = 'https://sample-app.203-0-113-5.sslip.io'
  const direct = {
    kind: 'public',
    url: 'http://203.0.113.5:8080',
    reachable_hint: 'reachable',
  }
  const reset = () => {
    httpsState.data = { state: 'off' }
    networkState.data = { fallback_url: fallback }
    urlsState.data = { addresses: [] }
  }
  const href = (text: string) => screen.getByText(text).closest('a')

  it('prefers a custom domain over everything', () => {
    reset()
    urlsState.data = { addresses: [direct] }
    renderPhase('live', { ...app, domains: ['app.example.com'] })
    expect(href('https://app.example.com')).toBeInTheDocument()
    expect(screen.queryByText(/certificate/)).not.toBeInTheDocument()
  })

  it('prefers a direct address over the https fallback', () => {
    reset()
    urlsState.data = { addresses: [direct] }
    renderPhase('live')
    expect(href(direct.url)).toBeInTheDocument()
    expect(screen.queryByText(/certificate/)).not.toBeInTheDocument()
  })

  it('uses the plain-http route before the https fallback', () => {
    reset()
    urlsState.data = {
      addresses: [],
      http_fallback_url: 'http://sample-app.203-0-113-5.sslip.io',
    }
    renderPhase('live')
    expect(href('http://sample-app.203-0-113-5.sslip.io')).toBeInTheDocument()
  })

  it('falls back to https with a non-alarming certificate note', () => {
    reset()
    renderPhase('live')
    expect(href(fallback)).toBeInTheDocument()
    expect(
      screen.getByText(getStartedEn.first.status.certNote),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: /https/i }),
    ).not.toBeInTheDocument()
  })

  it('drops the note once HTTPS is issued', () => {
    reset()
    httpsState.data = { state: 'issued' }
    renderPhase('live')
    expect(href(fallback)).toBeInTheDocument()
    expect(screen.queryByText(/certificate/)).not.toBeInTheDocument()
  })
})

describe('FirstAppStatus failed state', () => {
  it('shows the failure cause and fix with a retry and logs link', () => {
    attemptsState.data = [
      {
        failure: {
          cause: 'Image pull failed',
          suggested_fix: 'Check the registry credentials',
          retryable: true,
          deploy_id: 'dep_1',
        },
      },
    ]
    renderPhase('failed')
    expect(screen.getByText('Image pull failed')).toBeInTheDocument()
    expect(
      screen.getByText('Check the registry credentials'),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: getStartedEn.first.status.viewLogs }),
    ).toBeInTheDocument()
    screen
      .getByRole('button', { name: getStartedEn.first.status.retry })
      .click()
    expect(redeploy).toHaveBeenCalled()
  })
})

describe('FirstAppStatus touch targets', () => {
  it('gives the dashboard button a 44px minimum height on phones', () => {
    window.history.pushState({}, '', '/get-started')
    render(
      <I18nextProvider i18n={i18n}>
        <FirstAppStatus app={app} phase="live" onDone={vi.fn()} />
      </I18nextProvider>,
    )
    const done = screen.getByRole('button', {
      name: getStartedEn.first.status.done,
    })
    expect(done.className).toContain('max-sm:min-h-11')
  })
})

describe('FirstAppStatus on the dashboard', () => {
  it('hides the dashboard shortcut when already there', () => {
    window.history.pushState({}, '', '/')
    render(
      <I18nextProvider i18n={i18n}>
        <FirstAppStatus app={app} phase="live" onDone={vi.fn()} />
      </I18nextProvider>,
    )
    expect(
      screen.queryByRole('button', { name: getStartedEn.first.status.done }),
    ).not.toBeInTheDocument()
  })

  it('keeps the shortcut on other pages', () => {
    window.history.pushState({}, '', '/get-started')
    render(
      <I18nextProvider i18n={i18n}>
        <FirstAppStatus app={app} phase="live" onDone={vi.fn()} />
      </I18nextProvider>,
    )
    expect(
      screen.getByRole('button', { name: getStartedEn.first.status.done }),
    ).toBeInTheDocument()
    window.history.pushState({}, '', '/')
  })
})
