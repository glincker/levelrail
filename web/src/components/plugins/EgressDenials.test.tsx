import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import type { ReactNode } from 'react'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it } from 'vitest'
import en from '../../locales/en/plugins.json'
import type { Plugin } from '../../queries/plugins'
import { EgressDenials } from './EgressDenials'

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

function plugin(mode: 'open' | 'proxy', total: number): Plugin {
  return {
    name: 'p',
    egress_mode: mode,
    egress_denials: {
      total,
      by_reason: total ? { host_not_allowed: 3, blocked_address: 1 } : {},
      last_denied: total ? new Date().toISOString() : undefined,
    },
  } as Plugin
}

describe('EgressDenials', () => {
  it('shows a calm state with zero denials', () => {
    wrap(<EgressDenials plugin={plugin('proxy', 0)} />)
    expect(screen.getByText('No denied connections')).toBeTruthy()
  })

  it('summarises total, top reasons and last time', () => {
    wrap(<EgressDenials plugin={plugin('proxy', 4)} />)
    expect(screen.getByText('Denied connections: 4')).toBeTruthy()
    expect(screen.getByText(/host not declared \(3\)/)).toBeTruthy()
    expect(screen.getByText(/last /)).toBeTruthy()
  })

  it('renders nothing in open mode', () => {
    const { container } = wrap(<EgressDenials plugin={plugin('open', 4)} />)
    expect(container.firstChild).toBeNull()
  })
})
