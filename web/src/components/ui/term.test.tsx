import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import glossary from '../../locales/en/glossary.json'
import { Term } from './term'

vi.mock('../../hooks/useBrand', () => ({
  useBrand: () => ({ DocsURL: 'https://test.example/docs' }),
}))
vi.mock('virtual:docs-path-index', () => ({ default: new Set<string>() }))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['glossary'],
  resources: { en: { glossary } },
})

describe('Term', () => {
  it('shows the plain explanation and a learn more link when tapped', async () => {
    render(
      <I18nextProvider i18n={i18n}>
        <Term term="replica">Replicas</Term>
      </I18nextProvider>,
    )
    await userEvent.click(screen.getByRole('button', { name: 'Replicas' }))
    expect(await screen.findByText(/Identical copies/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Learn more/ })).toHaveAttribute(
      'href',
      'https://test.example/docs/autoscaling',
    )
  })
})
