import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { NoticeRows } from './DashboardBanners'
import { useDashboardNotices } from '../../hooks/useDashboardNotices'
import type { DoctorCheck } from '../../queries/systemDoctor'
import commonEn from '../../locales/en/common.json'
import dashboardEn from '../../locales/en/dashboard.json'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['dashboard', 'common'],
  defaultNS: 'dashboard',
  fallbackNS: 'common',
  resources: { en: { dashboard: dashboardEn, common: commonEn } },
  interpolation: { escapeValue: false },
})

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
vi.mock('../../hooks/useIsRoot', () => ({ useIsRoot: () => true }))

const HTTPS_TITLE = 'This dashboard is on plain HTTP'
const DOCTOR_TITLE_ONE = 'Doctor found 1 problem'

function failingCheck(code: string, name: string): DoctorCheck {
  return {
    code,
    name,
    status: 'fail',
    message: `${name} is failing`,
    class: 'incident',
  }
}

function stubApi(doctorChecks: DoctorCheck[]) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      const body = routeBody(String(url), doctorChecks)
      return Promise.resolve({
        ok: body !== undefined,
        status: body !== undefined ? 200 : 404,
        json: () => Promise.resolve(body),
        text: () => Promise.resolve(JSON.stringify(body ?? {})),
      })
    }),
  )
}

function routeBody(url: string, doctorChecks: DoctorCheck[]): unknown {
  if (url === '/api/v1/settings/ingress/https') {
    return {
      state: 'off',
      staging: false,
      suggested_domain: '203-0-113-7.sslip.io',
    }
  }
  if (url === '/api/v1/system/doctor') {
    return { ok: doctorChecks.length === 0, checks: doctorChecks }
  }
  if (url === '/api/v1/updates') {
    return { update_available: false }
  }
  return undefined
}

function Notices() {
  return <NoticeRows notices={useDashboardNotices()} />
}

function renderBanners() {
  const qc = createTestQueryClient()
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={qc}>
        <Notices />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('dashboard notices', () => {
  it('shows only the HTTPS banner when the doctor failure is the dashboard URL check', async () => {
    stubApi([failingCheck('dashboard_url_https', 'Dashboard URL uses HTTPS')])
    renderBanners()

    expect(await screen.findByText(HTTPS_TITLE)).toBeInTheDocument()
    await waitFor(() =>
      expect(fetch).toHaveBeenCalledWith('/api/v1/system/doctor'),
    )
    expect(screen.queryByText(DOCTOR_TITLE_ONE)).not.toBeInTheDocument()
  })

  it('shows both the HTTPS and the doctor banner for a different failing check', async () => {
    stubApi([failingCheck('disk_free', 'Free disk space')])
    renderBanners()

    expect(await screen.findByText(HTTPS_TITLE)).toBeInTheDocument()
    expect(await screen.findByText(DOCTOR_TITLE_ONE)).toBeInTheDocument()
  })
})
