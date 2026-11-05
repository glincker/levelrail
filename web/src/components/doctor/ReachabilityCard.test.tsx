import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import { ReachabilityCard } from './ReachabilityCard'
import domainsEn from '../../locales/en/domains.json'
import type { ReachabilityReport } from '../../queries/doctorReachability'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['domains'],
  defaultNS: 'domains',
  resources: { en: { domains: domainsEn } },
  interpolation: { escapeValue: false },
})

const report: ReachabilityReport = {
  status: 'warn',
  message: 'not reachable from the internet: https',
  dialback_enabled: false,
  ports: [
    {
      name: 'https',
      port: 443,
      listening: true,
      probes: [],
      external: 'unreachable',
      advice: 'Open port 443 inbound in your cloud firewall',
    },
  ],
}

const hook = vi.hoisted(() => ({ value: {} }))
vi.mock('../../queries/doctorReachability', () => ({
  useDoctorReachability: () => hook.value,
}))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => (
    <a href="/">{children}</a>
  ),
}))

function renderIt() {
  render(
    <I18nextProvider i18n={testI18n}>
      <ReachabilityCard />
    </I18nextProvider>,
  )
}

describe('ReachabilityCard', () => {
  it('shows a blocked port with its next step and a copy button', () => {
    hook.value = {
      data: report,
      isPending: false,
      isFetching: false,
      refetch: vi.fn(),
    }
    renderIt()
    expect(
      screen.getByText('Open port 443 inbound in your cloud firewall'),
    ).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Copy' })).toBeTruthy()
    expect(screen.getByText('Blocked from the internet')).toBeTruthy()
    expect(screen.getByText(/Internet check is off/)).toBeTruthy()
  })
})
