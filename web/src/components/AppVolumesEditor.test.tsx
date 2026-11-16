import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { AppVolumesEditor } from './AppVolumesEditor'
import appFilesEn from '../locales/en/appFiles.json'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('../hooks/useRestartRequiredToast', () => ({
  useRestartRequiredToast: () => vi.fn(),
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['appFiles'],
  defaultNS: 'appFiles',
  resources: { en: { appFiles: appFilesEn } },
  interpolation: { escapeValue: false },
})

function renderEditor(volumes: { name: string; container_path: string }[]) {
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={createTestQueryClient()}>
        <AppVolumesEditor appName="web" volumes={volumes} />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('AppVolumesEditor states', () => {
  it('shows one empty state and reveals the form on Add volume', async () => {
    renderEditor([])
    expect(screen.getByText('No volumes')).toBeInTheDocument()
    expect(screen.queryByLabelText('Volume name')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Add volume' }))
    expect(screen.getByLabelText('Volume name')).toBeInTheDocument()
  })

  it('shows the full editor when a volume exists', () => {
    renderEditor([{ name: 'data', container_path: '/data' }])
    expect(screen.getByLabelText('Volume name')).toBeInTheDocument()
    expect(screen.queryByText('No volumes')).not.toBeInTheDocument()
  })
})
