import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import dataExportEn from '../../locales/en/dataExport.json'
import { createTestQueryClient } from '@/test/testQueryClient'
import { CreateDataExportDestinationDialog } from './CreateDataExportDestinationDialog'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['dataExport'],
  defaultNS: 'dataExport',
  resources: { en: { dataExport: dataExportEn } },
  interpolation: { escapeValue: false },
})

function renderDialog(initialKind?: 'logs_loki') {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <I18nextProvider i18n={i18n}>
        <CreateDataExportDestinationDialog
          open
          onOpenChange={() => undefined}
          initialKind={initialKind}
        />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('CreateDataExportDestinationDialog', () => {
  it('goes from the gallery to a type form with advanced collapsed', async () => {
    renderDialog()
    await userEvent.click(screen.getByRole('button', { name: /Logs to Loki/ }))
    expect(screen.getByLabelText('Endpoint')).toBeInTheDocument()
    expect(screen.getByLabelText('Tenant ID (optional)')).toBeInTheDocument()
    expect(screen.getByLabelText('Name')).toHaveValue('logs-loki')
    const advanced = screen.getByText('Advanced').closest('details')
    expect(advanced).not.toHaveAttribute('open')
  })

  it('opens straight on the form for a deep link', () => {
    renderDialog('logs_loki')
    expect(screen.getByLabelText('Endpoint')).toBeInTheDocument()
  })
})
