import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it } from 'vitest'
import { TemplateCapabilitiesRow } from './TemplateCapabilitiesRow'
import templatesEn from '../../locales/en/templates.json'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['templates'],
  defaultNS: 'templates',
  resources: { en: { templates: templatesEn } },
  interpolation: { escapeValue: false },
})

function wrap(ui: React.ReactNode) {
  return render(<I18nextProvider i18n={testI18n}>{ui}</I18nextProvider>)
}

describe('TemplateCapabilitiesRow', () => {
  it('lists each grant with its tier, service and reason', () => {
    wrap(
      <TemplateCapabilitiesRow
        capabilities={[
          {
            class: 'cap_add',
            tier: 'review',
            service: 'web',
            reason: 'Binds port 80.',
            caps: ['NET_BIND_SERVICE'],
          },
          {
            class: 'net_admin',
            tier: 'elevated',
            service: 'vpn',
            reason: 'Creates a tunnel.',
            caps: ['NET_ADMIN'],
          },
        ]}
      />,
    )
    expect(screen.getAllByRole('listitem')).toHaveLength(2)
    expect(screen.getByText('Review')).toBeTruthy()
    expect(screen.getByText('Elevated')).toBeTruthy()
    expect(screen.getByText(/vpn: NET_ADMIN/)).toBeTruthy()
    expect(screen.getByText(/Creates a tunnel/)).toBeTruthy()
  })

  it('renders nothing without grants', () => {
    const { container } = wrap(<TemplateCapabilitiesRow capabilities={[]} />)
    expect(container.textContent).toBe('')
  })
})
