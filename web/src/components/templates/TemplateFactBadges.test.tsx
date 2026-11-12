import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { createInstance } from 'i18next'
import { describe, expect, it, vi } from 'vitest'
import templates from '../../locales/en/templates.json'
import { filterTemplates } from '../../lib/templateCatalogView'
import { TemplateFactBadges } from './TemplateFactBadges'

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    children,
    params,
    ...rest
  }: {
    children?: ReactNode
    params?: Record<string, string>
  } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
    <a href={`/templates/${params?.id ?? ''}`} {...rest}>
      {children}
    </a>
  ),
}))

async function renderBadges(node: ReactNode) {
  const i18n = createInstance()
  await i18n
    .use(initReactI18next)
    .init({ lng: 'en', resources: { en: { templates } } })
  return render(<I18nextProvider i18n={i18n}>{node}</I18nextProvider>)
}

describe('TemplateFactBadges', () => {
  it('flags a preview template with its hint', async () => {
    await renderBadges(<TemplateFactBadges facts={{ preview: true }} />)
    expect(screen.getByText('Preview').getAttribute('title')).toBe(
      templates.facts.preview.hint,
    )
  })

  it('renders nothing for an unknown license and no other facts', async () => {
    const { container } = await renderBadges(<TemplateFactBadges facts={{}} />)
    expect(container.textContent).toBe('')
  })

  it('states what was verified without a date', async () => {
    await renderBadges(
      <TemplateFactBadges
        full
        facts={{
          license: { spdx: 'MIT', class: 'permissive' },
          verification: { tag: '1.2.3', arches: ['arm64'] },
        }}
      />,
    )
    expect(screen.getByText('Permissive license')).toBeTruthy()
    expect(
      screen.getByText('Verified on arm64 with image tag 1.2.3'),
    ).toBeTruthy()
  })

  it('links a deprecated entry to its replacement', async () => {
    await renderBadges(
      <TemplateFactBadges
        full
        facts={{
          status: 'deprecated',
          status_reason: 'No longer maintained.',
          replaced_by: 'newer',
        }}
      />,
    )
    expect(screen.getByText('Deprecated')).toBeTruthy()
    expect(
      screen
        .getByRole('link', { name: 'Use newer instead' })
        .getAttribute('href'),
    ).toBe('/templates/newer')
  })
})

describe('filterTemplates facets', () => {
  const base = { slogan: '', category: 'X', documentation_url: '' }
  const list = [
    {
      ...base,
      id: 'a',
      name: 'A',
      requires_configuration: false,
      license: { spdx: 'MIT', class: 'permissive' as const },
      arch: { amd64: 'tested' as const, arm64: 'absent' as const },
    },
    { ...base, id: 'b', name: 'B', requires_configuration: false },
  ]
  it('hides entries with unknown facts once a facet is set', () => {
    const ids = (f: Parameters<typeof filterTemplates>[4]) =>
      filterTemplates(list, '', 'All', false, f).map((t) => t.id)
    expect(ids({ licenseClass: 'permissive' })).toEqual(['a'])
    expect(ids({ arch: 'amd64' })).toEqual(['a'])
    expect(ids({ arch: 'arm64' })).toEqual([])
    expect(ids({})).toEqual(['a', 'b'])
  })
})
