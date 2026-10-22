import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import commonEn from '../../locales/en/common.json'
import getStartedEn from '../../locales/en/getStarted.json'
import type { SetupGuide, SetupGuideStep } from '../../lib/setupGuide'
import { GetStartedHub } from './GetStartedHub'
import { StepCard } from './StepCard'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))
vi.mock('../../hooks/useIsRoot', () => ({ useIsRoot: () => true }))
vi.mock('../CreateResourceWizard', () => ({
  CreateResourceWizard: ({ trigger }: { trigger: React.ReactNode }) => trigger,
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['common', 'getStarted'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, getStarted: getStartedEn } },
  interpolation: { escapeValue: false },
})

const step = (
  over: Partial<SetupGuideStep> & Pick<SetupGuideStep, 'id'>,
): SetupGuideStep => ({
  title: over.id,
  state: 'todo',
  optional: false,
  estimate_minutes: 3,
  href: '/somewhere',
  ...over,
})

function guide(
  steps: SetupGuideStep[],
  next?: SetupGuide['next_step'],
): SetupGuide {
  return {
    steps,
    ready_percent: 40,
    done: steps.filter((s) => s.state === 'done').length,
    total: steps.length,
    next_step: next,
    complete: false,
    hidden: false,
    server_address: '203.0.113.7',
  }
}

function renderHub(client = new QueryClient()) {
  return render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <GetStartedHub />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('GetStartedHub', () => {
  it('shows live readiness, the next step and the server address', async () => {
    const g = guide(
      [
        step({ id: 'secure_admin', state: 'done' }),
        step({ id: 'first_app' }),
        step({ id: 'app_domain', blocked_by: ['first_app'] }),
      ],
      'first_app',
    )
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(g)))
    renderHub()
    expect(
      await screen.findByText('40% ready for production'),
    ).toBeInTheDocument()
    expect(screen.getByText('Ship your first app')).toBeInTheDocument()
    expect(screen.getByText(/203\.0\.113\.7/)).toBeInTheDocument()
    expect(
      screen.getByText(/Do "Deploy your first app" first/),
    ).toBeInTheDocument()
  })

  it('lists shipping the first app before the production steps', async () => {
    const g = guide(
      [
        step({ id: 'public_address' }),
        step({ id: 'secure_admin' }),
        step({ id: 'first_app' }),
        step({ id: 'backup_target' }),
      ],
      'first_app',
    )
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(g)))
    renderHub()
    const ship = await screen.findByText('Ship your first app')
    const ready = screen.getByText('Make it production ready')
    expect(
      ship.compareDocumentPosition(ready) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy()
    const deploy = screen.getByText('Deploy your first app')
    const address = screen.getByText(
      'Set the dashboard address and turn on HTTPS',
    )
    expect(
      deploy.compareDocumentPosition(address) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy()
  })

  it('flips a step to skipped without a reload and offers undo', async () => {
    const before = guide([step({ id: 'teammate', optional: true })])
    const after = guide([
      step({ id: 'teammate', optional: true, state: 'skipped' }),
    ])
    const fetchMock = vi.fn((_url: string, init?: RequestInit) =>
      Promise.resolve(jsonResponse(init?.method === 'PUT' ? after : before)),
    )
    vi.stubGlobal('fetch', fetchMock)
    renderHub()

    fireEvent.click(await screen.findByRole('button', { name: 'Skip' }))

    expect(
      await screen.findByRole('button', { name: 'Undo' }),
    ).toBeInTheDocument()
    const put = fetchMock.mock.calls.find(([, init]) => init?.method === 'PUT')
    expect(put?.[0]).toBe('/api/v1/setup/guide/steps/teammate')
    expect(JSON.parse(put?.[1]?.body as string)).toEqual({ state: 'skipped' })
    await waitFor(() => {
      expect(screen.getAllByText('Skipped').length).toBeGreaterThan(0)
    })
  })

  it('explains a load failure and offers a retry', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(new Response('boom', { status: 500 })),
    )
    renderHub()
    expect(
      await screen.findByRole('button', { name: 'Try again' }),
    ).toBeInTheDocument()
  })
})

describe('StepCard', () => {
  const base = {
    canEdit: true,
    onSkip: vi.fn(),
    onUndo: vi.fn(),
    onMarkDone: vi.fn(),
  }
  const wrap = (node: React.ReactNode) => (
    <I18nextProvider i18n={i18n}>
      <ul>{node}</ul>
    </I18nextProvider>
  )

  it('hides override actions for a non-admin', () => {
    render(
      wrap(<StepCard {...base} canEdit={false} step={step({ id: 'tls' })} />),
    )
    expect(screen.queryByRole('button', { name: 'Skip' })).toBeNull()
    expect(
      screen.getByRole('link', { name: /Review certificates/ }),
    ).toHaveAttribute('href', '/somewhere')
  })

  it('shows the reason for a step that is not needed', () => {
    render(
      wrap(
        <StepCard
          {...base}
          step={step({
            id: 'tls',
            state: 'not_applicable',
            reason: 'tls_terminated_upstream',
          })}
        />,
      ),
    )
    expect(screen.getByText(/proxy terminates TLS/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Skip' })).toBeNull()
  })

  it('offers undo for a manually completed step', () => {
    render(
      wrap(
        <StepCard
          {...base}
          step={step({ id: 'teammate', state: 'done', manual: true })}
        />,
      ),
    )
    expect(screen.getByRole('button', { name: 'Undo' })).toBeInTheDocument()
  })
})
