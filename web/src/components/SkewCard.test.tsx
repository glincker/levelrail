import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { SkewCard } from './SkewCard'
import resourcesEn from '../locales/en/resources.json'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['resources'],
  defaultNS: 'resources',
  resources: { en: { resources: resourcesEn } },
  interpolation: { escapeValue: false },
})

vi.mock('../hooks/useIsRoot', () => ({ useIsRoot: () => false }))
vi.mock('../queries/runtimeSettings', () => ({
  useRuntimeSettings: () => ({
    data: {
      settings: [
        { key: 'deploy.next_deployment_id', value: true },
        { key: 'deploy.next_asset_grace_seconds', value: 1800 },
        { key: 'deploy.next_asset_grace_max_held', value: 1 },
      ],
    },
  }),
  useSetRuntimeSetting: () => ({ mutate: vi.fn(), isPending: false }),
}))

describe('SkewCard', () => {
  it('shows the saved values read-only for a non-admin', () => {
    render(
      <I18nextProvider i18n={testI18n}>
        <SkewCard />
      </I18nextProvider>,
    )
    expect(screen.getByLabelText(/Old asset grace/)).toHaveValue(1800)
    expect(screen.getByLabelText(/Old asset grace/)).toBeDisabled()
  })
})
