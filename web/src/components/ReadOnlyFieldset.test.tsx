import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '../locales/en/common.json'
import { ReadOnlyFieldset } from './ReadOnlyFieldset'

interface AbilityState {
  role: string | null
  can: (ability: string) => boolean
}

const abilities = vi.hoisted((): { value: AbilityState } => ({
  value: { role: null, can: () => false },
}))

vi.mock('../hooks/useAbilities', () => ({
  useAbilities: () => abilities.value,
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { common } },
  interpolation: { escapeValue: false },
})

function renderGate() {
  render(
    <I18nextProvider i18n={testI18n}>
      <ReadOnlyFieldset ability="write:sensitive">
        <button type="button">Save</button>
      </ReadOnlyFieldset>
    </I18nextProvider>,
  )
}

describe('ReadOnlyFieldset', () => {
  beforeEach(() => {
    abilities.value = { role: null, can: () => false }
  })

  it('disables controls and shows one notice for a viewer', () => {
    abilities.value = { role: 'viewer', can: (a) => a === 'read' }
    renderGate()
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
    expect(screen.getAllByText(common.readOnlyNotice)).toHaveLength(1)
  })

  it('leaves controls enabled and hides the notice when allowed', () => {
    abilities.value = { role: 'operator', can: () => true }
    renderGate()
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
    expect(screen.queryByText(common.readOnlyNotice)).toBeNull()
  })

  it('stays disabled without a notice while the role is unknown', () => {
    renderGate()
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
    expect(screen.queryByText(common.readOnlyNotice)).toBeNull()
  })
})
