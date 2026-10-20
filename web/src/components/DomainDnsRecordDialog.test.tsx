import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import { DomainDnsRecordDialog } from './DomainDnsRecordDialog'
import domainsEn from '../locales/en/domains.json'

vi.mock('./DomainDnsCheck', () => ({
  DomainDnsCheck: ({
    appName,
    domain,
  }: {
    appName: string
    domain: string
  }) => <div data-testid="dns-check">{`${appName}:${domain}`}</div>,
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['domains'],
  defaultNS: 'domains',
  resources: { en: { domains: domainsEn } },
  interpolation: { escapeValue: false },
})

function renderDialog(onClose = vi.fn()) {
  render(
    <I18nextProvider i18n={testI18n}>
      <DomainDnsRecordDialog
        appName="web"
        domain="app.example.com"
        onClose={onClose}
      />
    </I18nextProvider>,
  )
  return onClose
}

describe('DomainDnsRecordDialog', () => {
  it('titles the dialog with the domain and renders the check for that app and domain', () => {
    renderDialog()
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(
      screen.getByRole('heading', { name: /app\.example\.com/ }),
    ).toBeInTheDocument()
    expect(screen.getByTestId('dns-check')).toHaveTextContent(
      'web:app.example.com',
    )
  })

  it('calls onClose when dismissed with Escape', async () => {
    const onClose = renderDialog()
    await userEvent.keyboard('{Escape}')
    expect(onClose).toHaveBeenCalled()
  })
})
