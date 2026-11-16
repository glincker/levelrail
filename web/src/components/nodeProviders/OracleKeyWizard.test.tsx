import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import nodesEn from '../../locales/en/nodes.json'
import { OracleKeyWizard } from './OracleKeyWizard'

vi.mock('@/lib/ociKey', () => ({
  generateOciKeyPair: () =>
    Promise.resolve({
      publicKeyPem:
        '-----BEGIN PUBLIC KEY-----\nPUB\n-----END PUBLIC KEY-----\n',
      privateKeyPem:
        '-----BEGIN PRIVATE KEY-----\nSECRETBODY\n-----END PRIVATE KEY-----\n',
      fingerprint: 'aa:bb',
    }),
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['nodes'],
  defaultNS: 'nodes',
  resources: { en: { nodes: nodesEn } },
  interpolation: { escapeValue: false },
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe('OracleKeyWizard', () => {
  it('hands the private key to the form only and never persists it', async () => {
    const setItem = vi.spyOn(Storage.prototype, 'setItem')
    const onUseKey = vi.fn()
    render(
      <QueryClientProvider client={new QueryClient()}>
        <I18nextProvider i18n={testI18n}>
          <OracleKeyWizard connected={false} onUseKey={onUseKey} />
        </I18nextProvider>
      </QueryClientProvider>,
    )
    fireEvent.click(screen.getByRole('button', { name: /guided api key/i }))
    fireEvent.click(
      await screen.findByRole('button', { name: 'Generate key pair' }),
    )
    fireEvent.click(
      await screen.findByRole('button', { name: 'Use key in the form' }),
    )
    expect(onUseKey).toHaveBeenCalledWith(
      expect.stringContaining('SECRETBODY'),
      'aa:bb',
    )
    expect(document.body.textContent).not.toContain('SECRETBODY')
    expect(setItem).not.toHaveBeenCalled()
    expect(
      screen
        .getByRole('button', { name: 'Check connection' })
        .hasAttribute('disabled'),
    ).toBe(true)
  })
})
