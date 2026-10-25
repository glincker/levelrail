import { describe, expect, it } from 'vitest'
import { render as rtlRender, screen } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import type { ReactElement } from 'react'
import templatesEn from '../../locales/en/templates.json'
import { TemplateServices } from './TemplateServices'
import { TemplateTrustBadges } from './TemplateTrustBadges'
import type { ComposePreviewService } from '../../types/compose'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['templates'],
  defaultNS: 'templates',
  resources: { en: { templates: templatesEn } },
  interpolation: { escapeValue: false },
})

function render(ui: ReactElement) {
  return rtlRender(<I18nextProvider i18n={testI18n}>{ui}</I18nextProvider>)
}

const services: ComposePreviewService[] = [
  {
    key: 'web',
    image: 'ghcr.io/acme/web:1.2',
    port: 8080,
    volumes: ['data'],
    env: [
      { key: 'DB_PASSWORD', secret: true, generated: true },
      { key: 'ADMIN_EMAIL', required: true },
      { key: 'LOG_LEVEL', value: 'info' },
    ],
  },
]

describe('TemplateServices', () => {
  it('labels env names by who supplies them and never shows values', () => {
    render(<TemplateServices services={services} />)
    expect(screen.getByText('DB_PASSWORD')).toBeInTheDocument()
    expect(screen.getByText('generated')).toBeInTheDocument()
    expect(screen.getByText('you provide')).toBeInTheDocument()
    expect(screen.queryByText('info')).not.toBeInTheDocument()
    expect(screen.getByText('data')).toBeInTheDocument()
  })
})

describe('TemplateTrustBadges', () => {
  it('renders nothing without images', () => {
    const { container } = render(<TemplateTrustBadges images={[]} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('flags floating tags', () => {
    render(<TemplateTrustBadges images={['nginx:latest', 'a/b:2']} />)
    expect(screen.getByText(/floating tag/)).toBeInTheDocument()
    expect(screen.getByText(/version tag/)).toBeInTheDocument()
  })
})
