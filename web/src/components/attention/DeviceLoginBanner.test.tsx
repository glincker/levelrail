import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { DeviceLoginBanner } from './DeviceLoginBanner'
import attentionEn from '../../locales/en/attention.json'
import type {
  DeviceActivityItem,
  DeviceAuthRequest,
} from '../../queries/deviceAuth'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['attention'],
  defaultNS: 'attention',
  resources: { en: { attention: attentionEn } },
  interpolation: { escapeValue: false },
})

const approveMutate = vi.fn()
const denyMutate = vi.fn()
let requests: DeviceAuthRequest[] | undefined
let activity: DeviceActivityItem[] | undefined
const dismissMutate = vi.fn()

vi.mock('../../queries/deviceAuth', () => ({
  useLiveDeviceAuthRequests: () => ({ data: requests }),
  useDeviceActivity: () => ({ data: activity }),
  useDismissAttentionItem: () => ({
    mutate: dismissMutate,
    isPending: false,
  }),
  useApproveDeviceAuthRequest: () => ({
    mutate: approveMutate,
    isPending: false,
  }),
  useDenyDeviceAuthRequest: () => ({ mutate: denyMutate, isPending: false }),
}))
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

function request(over: Partial<DeviceAuthRequest>): DeviceAuthRequest {
  return {
    user_code: 'ABCD-1234',
    client_name: 'laptop',
    created_at: new Date(Date.now() - 60_000).toISOString(),
    expires_at: new Date(Date.now() + 5 * 60_000).toISOString(),
    requester_ip: '203.0.113.9',
    user_agent: 'levelrail-cli/1.0',
    ip_mismatch: false,
    ...over,
  }
}

function renderBanner() {
  return render(
    <I18nextProvider i18n={testI18n}>
      <DeviceLoginBanner />
    </I18nextProvider>,
  )
}

afterEach(() => {
  requests = undefined
  activity = undefined
  dismissMutate.mockReset()
  approveMutate.mockReset()
  denyMutate.mockReset()
})

describe('DeviceLoginBanner', () => {
  it('renders nothing when no login is waiting', () => {
    requests = []
    const { container } = renderBanner()
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing for a request that has expired', () => {
    requests = [
      request({ expires_at: new Date(Date.now() - 1000).toISOString() }),
    ]
    const { container } = renderBanner()
    expect(container).toBeEmptyDOMElement()
  })

  it('shows the code, requester and countdown with no dismiss control', () => {
    requests = [request({})]
    renderBanner()
    expect(screen.getByTestId('device-banner-code')).toHaveTextContent(
      'ABCD-1234',
    )
    expect(screen.getByText(/203\.0\.113\.9/)).toBeInTheDocument()
    expect(
      screen.getByText(/Expires in 4:5\d|Expires in 5:00/),
    ).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /dismiss|close/i })).toBeNull()
  })

  it('requires a confirm click and never approves on the first click', () => {
    requests = [request({})]
    renderBanner()
    fireEvent.click(screen.getByRole('button', { name: 'Review and approve' }))
    expect(approveMutate).not.toHaveBeenCalled()
    expect(screen.getByTestId('device-confirm-code')).toHaveTextContent(
      'ABCD-1234',
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'The codes match, approve' }),
    )
    expect(approveMutate).toHaveBeenCalledTimes(1)
    expect(approveMutate.mock.calls[0]?.[0]).toBe('ABCD-1234')
  })

  it('warns when the requesting IP differs from this session', () => {
    requests = [request({ ip_mismatch: true })]
    renderBanner()
    expect(
      screen.getByText(/not the address you are using/),
    ).toBeInTheDocument()
  })

  it('denies inline without a dialog', () => {
    requests = [request({})]
    renderBanner()
    fireEvent.click(screen.getByRole('button', { name: 'Deny' }))
    expect(denyMutate).toHaveBeenCalledTimes(1)
  })

  it('shows an expired login as a dismissable strip with the start over command', () => {
    requests = []
    activity = [
      {
        id: 'r1',
        state: 'expired',
        client_name: 'ci-box',
        requester_ip: '203.0.113.9',
        user_agent: '',
        created_at: new Date(Date.now() - 3_600_000).toISOString(),
        expires_at: new Date(Date.now() - 3_000_000).toISOString(),
        ip_mismatch: false,
        dismissible: true,
        dismissed: false,
        item_key: 'device_login:r1:expired',
        audit_path: '/api/v1/auth/device/requests/r1',
      },
    ]
    renderBanner()
    expect(screen.getByText(/ci-box expired/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Start over' }))
    expect(
      screen.getByText('levelrail-cli auth login --device'),
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }))
    expect(dismissMutate.mock.calls[0]?.[0]).toBe('device_login:r1:expired')
  })

  it('hides a dismissed login and never shows approved ones', () => {
    requests = []
    const base = {
      client_name: 'x',
      requester_ip: '',
      user_agent: '',
      created_at: new Date().toISOString(),
      expires_at: new Date().toISOString(),
      ip_mismatch: false,
      dismissible: true,
      audit_path: '/p',
    }
    activity = [
      { ...base, id: 'a', state: 'expired', dismissed: true, item_key: 'k1' },
      { ...base, id: 'b', state: 'approved', dismissed: false, item_key: 'k2' },
    ]
    const { container } = renderBanner()
    expect(container).toBeEmptyDOMElement()
  })
})
