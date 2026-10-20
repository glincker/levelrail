import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { ExistingVolumePrompt } from './ExistingVolumePrompt'
import databasesEn from '../locales/en/databases.json'

function renderPrompt(onChoose: (c: 'reuse' | 'discard') => void) {
  const i18n = i18next.createInstance()
  void i18n.use(initReactI18next).init({
    lng: 'en',
    resources: { en: { databases: databasesEn } },
    interpolation: { escapeValue: false },
  })
  render(
    <I18nextProvider i18n={i18n}>
      <ExistingVolumePrompt
        volumes={[
          { name: 'db-main-data', size_bytes: 3 * 1024 * 1024, mounted: false },
        ]}
        pending={false}
        onChoose={onChoose}
      />
    </I18nextProvider>,
  )
}

describe('ExistingVolumePrompt', () => {
  it('shows the old volume size and reports the explicit choice', async () => {
    const onChoose = vi.fn()
    renderPrompt(onChoose)
    expect(screen.getByText(/3\.0 MiB/)).toBeTruthy()

    await userEvent.click(
      screen.getByRole('button', { name: 'Discard and start empty' }),
    )
    expect(onChoose).toHaveBeenCalledWith('discard')

    await userEvent.click(
      screen.getByRole('button', { name: 'Reuse old data' }),
    )
    expect(onChoose).toHaveBeenCalledWith('reuse')
  })
})
