import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import { IngressUpstreamFields } from './IngressUpstreamFields'
import { isValidPublicPort } from '../lib/publicPort'
import domainsEn from '../locales/en/domains.json'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['domains'],
  defaultNS: 'domains',
  resources: { en: { domains: domainsEn } },
  interpolation: { escapeValue: false },
})

describe('isValidPublicPort', () => {
  it.each([
    ['', true],
    ['0', true],
    ['443', true],
    ['65535', true],
    ['65536', false],
    ['-1', false],
    ['80a', false],
  ])('%j -> %s', (raw, want) => {
    expect(isValidPublicPort(raw)).toBe(want)
  })
})

describe('IngressUpstreamFields', () => {
  it('shows the helper text, banner when upstream is saved, and reports edits', async () => {
    const onPortChange = vi.fn()
    render(
      <I18nextProvider i18n={testI18n}>
        <IngressUpstreamFields
          settings={{
            acme_enabled: false,
            hsts_enabled: false,
            tls_terminated_upstream: true,
          }}
          upstream
          onUpstreamChange={vi.fn()}
          port=""
          onPortChange={onPortChange}
        />
      </I18nextProvider>,
    )

    expect(screen.getByText('HTTPS is handled by your proxy')).toBeTruthy()
    expect(
      screen.getByText(/enter the port clients use \(usually 443\)/),
    ).toBeTruthy()
    await userEvent.type(screen.getByLabelText('Public HTTPS port'), '4')
    expect(onPortChange).toHaveBeenCalledWith('4')
  })

  it('renders the translated validation error', () => {
    render(
      <I18nextProvider i18n={testI18n}>
        <IngressUpstreamFields
          settings={{ acme_enabled: false, hsts_enabled: false }}
          upstream={false}
          onUpstreamChange={vi.fn()}
          port="70000"
          onPortChange={vi.fn()}
          invalid
        />
      </I18nextProvider>,
    )
    expect(screen.getByText(/between 0 and 65535/)).toBeTruthy()
  })
})
