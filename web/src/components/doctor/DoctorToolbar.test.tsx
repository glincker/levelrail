import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import { DoctorToolbar } from './DoctorToolbar'
import settingsEn from '../../locales/en/settings.json'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['settings'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn } },
  interpolation: { escapeValue: false },
})

function renderIt(onFilter = vi.fn(), running = false) {
  render(
    <I18nextProvider i18n={testI18n}>
      <DoctorToolbar
        counts={{ fail: 2, warn: 3, unknown: 0, ok: 5 }}
        total={10}
        filter="all"
        onFilter={onFilter}
        checkedAt={new Date('2026-01-01T10:00:00Z')}
        running={running}
        diagnostics={() => 'diag'}
      />
    </I18nextProvider>,
  )
  return onFilter
}

describe('DoctorToolbar', () => {
  it('shows counts and switches the filter', async () => {
    const onFilter = renderIt()
    expect(screen.getByRole('button', { name: /Failing/ })).toHaveTextContent(
      '2',
    )
    await userEvent.click(screen.getByRole('button', { name: /Warnings/ }))
    expect(onFilter).toHaveBeenCalledWith('warn')
    expect(screen.getByText(/Last checked/)).toBeInTheDocument()
  })

  it('disables copy while checks run', () => {
    renderIt(vi.fn(), true)
    expect(
      screen.getByRole('button', { name: /Copy diagnostics/ }),
    ).toBeDisabled()
    expect(screen.getByText('Checking...')).toBeInTheDocument()
  })
})
