import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import databaseAccess from '../../locales/en/databaseAccess.json'
import { NetworkPanel } from './NetworkPanel'

const network = vi.hoisted((): { canEdit: boolean | undefined } => ({
  canEdit: undefined,
}))

vi.mock('../../queries/databaseAccess', () => ({
  useDatabaseNetwork: () => ({
    data: { scope: { current: 'x' }, can_edit: network.canEdit },
    isLoading: false,
    error: null,
  }),
}))
vi.mock('./ReachabilityCard', () => ({ ReachabilityCard: () => null }))
vi.mock('./AllowedSourcesCard', () => ({ AllowedSourcesCard: () => null }))
vi.mock('./TlsCard', () => ({ TlsCard: () => null }))
vi.mock('./ScopeCard', () => ({
  ScopeCard: () => <button type="button">Save scope</button>,
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { databaseAccess } },
  defaultNS: 'databaseAccess',
  interpolation: { escapeValue: false },
})

function renderPanel() {
  render(
    <I18nextProvider i18n={testI18n}>
      <NetworkPanel databaseName="db" />
    </I18nextProvider>,
  )
}

describe('NetworkPanel read-only gate', () => {
  beforeEach(() => {
    network.canEdit = undefined
  })

  it('disables controls and explains it for a viewer', () => {
    network.canEdit = false
    renderPanel()
    expect(screen.getByRole('button', { name: 'Save scope' })).toBeDisabled()
    expect(
      screen.getByText(databaseAccess.network.readOnlyNotice),
    ).toBeInTheDocument()
  })

  it('enables controls and hides the notice for an admin', () => {
    network.canEdit = true
    renderPanel()
    expect(screen.getByRole('button', { name: 'Save scope' })).toBeEnabled()
    expect(screen.queryByText(databaseAccess.network.readOnlyNotice)).toBeNull()
  })

  it('fails closed when the server omits can_edit', () => {
    renderPanel()
    expect(screen.getByRole('button', { name: 'Save scope' })).toBeDisabled()
  })
})
