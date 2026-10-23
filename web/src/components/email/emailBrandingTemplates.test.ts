import { describe, expect, it } from 'vitest'
import {
  BRANDING_TEMPLATE_IDS,
  buildBrandingTemplate,
  KNOWN_BRANDING_VARIABLES,
  variablesIn,
} from './emailBrandingTemplates'
import settingsEn from '../../locales/en/settings.json'

const templates = settingsEn.emailBranding.templates

describe('branding templates', () => {
  it('only use variables from the closed registry', () => {
    for (const id of BRANDING_TEMPLATE_IDS) {
      const copy = templates[id]
      const text =
        'address' in copy ? `${copy.text} ${copy.address}` : copy.text
      for (const name of variablesIn(text)) {
        expect(KNOWN_BRANDING_VARIABLES).toContain(name)
      }
    }
  })

  it('drops a variable the server does not list instead of saving a refused token', () => {
    const patch = buildBrandingTemplate('standard', {
      copy: { text: 'Sent by {{brand_name}} {{year}}', dashboardLink: 'Open' },
      variables: ['brand_name'],
      currentColor: '',
    })
    expect(patch.footer_text).toBe('Sent by {{brand_name}}')
  })

  it('fills the accent color only when empty and adds the address line for compliance', () => {
    const base = {
      copy: {
        text: 'x',
        address: '{{brand_name}} street',
        dashboardLink: 'Open',
      },
      brandColor: '#112233',
      dashboardUrl: 'https://deploy.example.com',
    }
    const compliance = buildBrandingTemplate('compliance', {
      ...base,
      currentColor: '',
    })
    expect(compliance.brand_color).toBe('#112233')
    expect(compliance.footer_address).toBe('{{brand_name}} street')
    expect(compliance.footer_links).toHaveLength(1)

    const kept = buildBrandingTemplate('minimal', {
      ...base,
      currentColor: '#abcdef',
    })
    expect(kept.brand_color).toBeUndefined()
    expect(kept.footer_links).toEqual([])
    expect(kept).not.toHaveProperty('footer_address')
  })

  it('never links to a non-https dashboard', () => {
    const patch = buildBrandingTemplate('standard', {
      copy: { text: 'x', dashboardLink: 'Open' },
      currentColor: '',
      dashboardUrl: 'http://deploy.example.com',
    })
    expect(patch.footer_links).toEqual([])
  })
})
