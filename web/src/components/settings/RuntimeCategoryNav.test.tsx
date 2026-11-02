import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import en from '../../locales/en/runtimeSettings.json'
import { HighlightedText } from './HighlightedText'
import { RuntimeCategoryNav } from './RuntimeCategoryNav'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['runtimeSettings'],
  defaultNS: 'runtimeSettings',
  resources: { en: { runtimeSettings: en } },
  interpolation: { escapeValue: false },
})

describe('RuntimeCategoryNav', () => {
  it('selects a category and marks the active one', async () => {
    const onSelect = vi.fn()
    render(
      <I18nextProvider i18n={i18n}>
        <RuntimeCategoryNav
          counts={[
            { category: 'security', count: 3 },
            { category: 'deploys', count: 1 },
          ]}
          total={4}
          active="deploys"
          onSelect={onSelect}
        />
      </I18nextProvider>,
    )
    const active = screen.getByRole('button', { name: /Deploys/ })
    expect(active).toHaveAttribute('aria-current', 'true')
    await userEvent.click(screen.getByRole('button', { name: /Security/ }))
    expect(onSelect).toHaveBeenCalledWith('security')
    await userEvent.selectOptions(screen.getByRole('combobox'), '')
    expect(onSelect).toHaveBeenLastCalledWith(undefined)
  })
})

describe('HighlightedText', () => {
  it('marks matches', () => {
    render(<HighlightedText text="Trust proxy" query="prox" />)
    expect(screen.getByText('prox').tagName).toBe('MARK')
  })
})
