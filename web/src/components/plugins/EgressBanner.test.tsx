import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import type { ReactNode } from 'react'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it } from 'vitest'
import en from '../../locales/en/plugins.json'
import type { Plugin } from '../../queries/plugins'
import { EgressBanner } from './EgressBanner'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['plugins'],
  defaultNS: 'plugins',
  resources: { en: { plugins: en } },
  interpolation: { escapeValue: false },
})

function wrap(ui: ReactNode) {
  return render(<I18nextProvider i18n={i18n}>{ui}</I18nextProvider>)
}

function plugin(hosts: string[], mode: 'open' | 'proxy'): Plugin {
  return { name: 'p', outbound_hosts: hosts, egress_mode: mode } as Plugin
}

describe('EgressBanner', () => {
  it('warns when declared hosts are not enforced', () => {
    wrap(<EgressBanner plugins={[plugin(['a.example.com'], 'open')]} />)
    expect(screen.getByText(/not enforced/i)).toBeTruthy()
  })

  it('stays hidden in proxy mode or with no hosts', () => {
    const { container } = wrap(
      <EgressBanner
        plugins={[plugin(['a.example.com'], 'proxy'), plugin([], 'open')]}
      />,
    )
    expect(container.firstChild).toBeNull()
  })
})
