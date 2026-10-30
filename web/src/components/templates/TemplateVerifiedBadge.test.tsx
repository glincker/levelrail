import { render, screen } from '@testing-library/react'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { createInstance } from 'i18next'
import { describe, expect, it } from 'vitest'
import templates from '../../locales/en/templates.json'
import { filterTemplates } from '../../lib/templateCatalogView'
import { TemplateVerifiedBadge } from './TemplateVerifiedBadge'

describe('TemplateVerifiedBadge', () => {
  it('renders the translated label', async () => {
    const i18n = createInstance()
    await i18n
      .use(initReactI18next)
      .init({ lng: 'en', resources: { en: { templates } } })
    render(
      <I18nextProvider i18n={i18n}>
        <TemplateVerifiedBadge />
      </I18nextProvider>,
    )
    expect(screen.getByText('Verified')).toBeTruthy()
  })
})

describe('filterTemplates verifiedOnly', () => {
  const base = { slogan: '', category: 'X', documentation_url: '' }
  const list = [
    {
      ...base,
      id: 'a',
      name: 'A',
      requires_configuration: false,
      verified: true,
    },
    { ...base, id: 'b', name: 'B', requires_configuration: false },
  ]
  it('keeps only verified entries when on', () => {
    expect(filterTemplates(list, '', 'All', true).map((t) => t.id)).toEqual([
      'a',
    ])
    expect(filterTemplates(list, '', 'All').length).toBe(2)
  })
})
