import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import maintenanceEn from '../../locales/en/maintenance.json'
import { AppMaintenanceBanner } from './AppMaintenanceBanner'
import { DeployMaintenanceNotice } from './DeployMaintenanceNotice'

const state = vi.hoisted(() => ({
  data: undefined as
    | { enabled: boolean; active_domains: number; total_domains: number }
    | undefined,
}))
vi.mock('../../queries/appMaintenance', () => ({
  useAppMaintenance: () => ({ data: state.data }),
  useClearAppMaintenance: () => ({ mutate: vi.fn(), isPending: false }),
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['maintenance'],
  defaultNS: 'maintenance',
  resources: { en: { maintenance: maintenanceEn } },
  interpolation: { escapeValue: false },
})

function renderBoth() {
  return render(
    <I18nextProvider i18n={i18n}>
      <AppMaintenanceBanner appName="web" />
      <DeployMaintenanceNotice appName="web" />
    </I18nextProvider>,
  )
}

describe('app level maintenance UI', () => {
  it('shows nothing when the app is not in maintenance', () => {
    state.data = { enabled: false, active_domains: 0, total_domains: 2 }
    renderBoth()
    expect(screen.queryByText(/in maintenance/i)).toBeNull()
    expect(screen.queryByText(/Maintenance is on/)).toBeNull()
  })

  it('shows one app banner and the deploy notice when it is', () => {
    state.data = { enabled: true, active_domains: 2, total_domains: 2 }
    renderBoth()
    expect(
      screen.getAllByText(/Maintenance is on for 2 of 2 domains/),
    ).toHaveLength(1)
    expect(
      screen.getByText(
        'This app is in maintenance. The deploy will complete and maintenance stays on.',
      ),
    ).toBeTruthy()
  })
})
