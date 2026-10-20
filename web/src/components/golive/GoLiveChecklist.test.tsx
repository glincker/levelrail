import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import goliveEn from '../../locales/en/golive.json'
import type {
  ChecklistStep,
  GoLiveChecklistData,
  StepStatus,
} from '../../queries/goLiveChecklist'

const navigate = vi.fn()
vi.mock('@tanstack/react-router', () => ({
  useRouter: () => ({ navigate }),
  Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
}))

const stream = vi.hoisted(() => {
  const state: { current: unknown; refresh: ReturnType<typeof vi.fn> } = {
    current: undefined,
    refresh: vi.fn(),
  }
  return state
})
vi.mock('../../queries/goLiveChecklist', async (orig) => {
  const actual = await orig<typeof import('../../queries/goLiveChecklist')>()
  return {
    ...actual,
    useChecklistStream: () => ({
      checklist: stream.current,
      error: undefined,
      refresh: stream.refresh,
    }),
  }
})

import { GoLiveChecklist } from './GoLiveChecklist'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['golive'],
  defaultNS: 'golive',
  resources: { en: { golive: goliveEn } },
  interpolation: { escapeValue: false },
})

function step(
  id: ChecklistStep['id'],
  status: StepStatus,
  extra: Partial<ChecklistStep> = {},
): ChecklistStep {
  return {
    id,
    title: `title ${id}`,
    status,
    summary: `summary ${id}`,
    evidence: [],
    ...extra,
  }
}

function checklist(
  steps: ChecklistStep[],
  state: GoLiveChecklistData['state'] = 'needs_attention',
): GoLiveChecklistData {
  return {
    app: 'shop',
    domain: 'shop.dev',
    state,
    live: state === 'live',
    url: 'https://shop.dev',
    steps,
    timeline: [],
    checked_at: '2026-10-20T12:00:00Z',
  }
}

function renderChecklist() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={i18n}>
        <GoLiveChecklist app="shop" domain="shop.dev" />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

const firewallFix = {
  fix_id: 'host.firewall.open_web_ports',
  kind: 'auto',
  commands: ['ufw allow 80/tcp', 'ufw allow 443/tcp'],
  undo: ['ufw delete allow 80/tcp'],
  persistent: false,
}

beforeEach(() => {
  navigate.mockReset()
  stream.refresh.mockReset()
})
afterEach(() => {
  vi.unstubAllGlobals()
})

describe('GoLiveChecklist', () => {
  it('shows every step with an accessible status and one primary button per open step', () => {
    stream.current = checklist([
      step('app', 'ok'),
      step('port', 'ok'),
      step('firewall', 'fail', {
        primary: { kind: 'recheck', label: 'Check again' },
      }),
      step('dns', 'pending', {
        primary: { kind: 'recheck', label: 'Check again' },
      }),
      step('certificate', 'pending'),
      step('live', 'unknown', { scope: 'control_plane_host' }),
    ])
    renderChecklist()
    expect(
      screen.getAllByRole('heading', { level: 3 }).length,
    ).toBeGreaterThanOrEqual(6)
    expect(screen.getByText(/title firewall/)).toBeInTheDocument()
    expect(screen.getByText('(Needs action)')).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: 'Check again' })).toHaveLength(
      3,
    )
    expect(
      screen.getByText(/cannot prove the wider internet/),
    ).toBeInTheDocument()
  })

  it('does not offer a button on a finished step', () => {
    stream.current = checklist(
      [
        step('app', 'ok', {
          primary: { kind: 'recheck', label: 'Hidden action' },
        }),
      ],
      'live',
    )
    renderChecklist()
    expect(screen.queryByRole('button', { name: 'Hidden action' })).toBeNull()
    expect(screen.getByText('Open shop.dev').closest('a')).toHaveAttribute(
      'href',
      'https://shop.dev',
    )
  })

  it('confirms the firewall fix with commands and undo before applying it', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve({}),
    })
    vi.stubGlobal('fetch', fetchMock)
    stream.current = checklist([
      step('firewall', 'fail', {
        firewall_fix: firewallFix,
        primary: {
          kind: 'fix',
          label: 'Open ports 80 and 443',
          fix_id: firewallFix.fix_id,
        },
      }),
    ])
    renderChecklist()
    await userEvent.click(
      screen.getByRole('button', { name: 'Open ports 80 and 443' }),
    )
    expect(fetchMock).not.toHaveBeenCalled()
    expect(screen.getByText('Will run: ufw allow 80/tcp')).toBeInTheDocument()
    expect(
      screen.getByText('Undo with: ufw delete allow 80/tcp'),
    ).toBeInTheDocument()
    expect(screen.getByText(/lost on reboot/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Open ports' }))
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/fixes/host.firewall.open_web_ports/apply',
        expect.objectContaining({ method: 'POST' }),
      )
    })
    await waitFor(() => {
      expect(stream.refresh).toHaveBeenCalled()
    })
  })

  it('runs a request action directly and rechecks on the recheck button', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve({}),
    })
    vi.stubGlobal('fetch', fetchMock)
    stream.current = checklist([
      step('certificate', 'pending', {
        primary: {
          kind: 'request',
          label: 'Get a real certificate',
          api: 'POST /api/v1/domains/shop.dev/cert',
          body: {},
        },
      }),
    ])
    renderChecklist()
    await userEvent.click(
      screen.getByRole('button', { name: 'Get a real certificate' }),
    )
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/domains/shop.dev/cert',
        expect.objectContaining({ method: 'POST' }),
      )
    })
    await userEvent.click(screen.getByRole('button', { name: 'Check again' }))
    expect(stream.refresh).toHaveBeenCalled()
  })

  it('renders provider guidance, records and a disabled blocked action', () => {
    stream.current = checklist([
      step('firewall', 'fail', {
        provider: {
          id: 'hetzner',
          name: 'Hetzner Cloud',
          docs_url: 'https://docs.hetzner.com/x',
          ports: [{ port: 80, protocol: 'tcp', source: '0.0.0.0/0' }],
          steps: ['Open Firewalls'],
        },
        primary: {
          kind: 'recheck',
          label: 'Check again',
          blocked: 'wait an hour',
        },
      }),
      step('dns', 'fail', {
        evidence: [
          {
            label: 'Record to create',
            value: 'shop.dev A 1.2.3.4',
            copy: true,
          },
        ],
      }),
    ])
    renderChecklist()
    expect(screen.getByText('Hetzner Cloud firewall steps')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Copy Record to create' }),
    ).toBeInTheDocument()
    const blocked = screen
      .getAllByRole('button', { name: 'Check again' })
      .find((b) => b.hasAttribute('disabled'))
    expect(blocked).toBeDefined()
    expect(
      screen.getByText('Not available yet: wait an hour'),
    ).toBeInTheDocument()
  })

  it('announces a status change in a polite live region', () => {
    stream.current = checklist([step('dns', 'pending')])
    const { rerender } = renderChecklist()
    stream.current = checklist([step('dns', 'ok')])
    rerender(
      <QueryClientProvider client={new QueryClient()}>
        <I18nextProvider i18n={i18n}>
          <GoLiveChecklist app="shop" domain="shop.dev" />
        </I18nextProvider>
      </QueryClientProvider>,
    )
    const region = document.querySelector('[aria-live="polite"]')
    expect(region?.textContent).toContain('title dns: Done')
  })
})
