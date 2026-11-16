import { render, screen, within } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it } from 'vitest'
import commonEn from '../../locales/en/common.json'
import { CardGrid, MetaRow, ObjectCard, ObjectCardSkeleton } from './ObjectCard'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { common: commonEn } },
  interpolation: { escapeValue: false },
})

describe('ObjectCard', () => {
  it('renders title as a heading, status, meta and footer actions', () => {
    render(
      <CardGrid>
        <li>
          <ObjectCard
            title="acme-org"
            status={<span>Healthy</span>}
            meta={['github.com', null, 'OAuth']}
            actions={<button type="button">Manage</button>}
          >
            <p>body</p>
          </ObjectCard>
        </li>
      </CardGrid>,
    )
    expect(
      screen.getByRole('heading', { level: 3, name: 'acme-org' }),
    ).toBeTruthy()
    expect(screen.getByText('Healthy')).toBeTruthy()
    expect(screen.getByText('body')).toBeTruthy()
    const card = screen
      .getByRole('heading', { name: 'acme-org' })
      .closest('[data-slot="object-card"]') as HTMLElement
    expect(within(card).getByRole('button', { name: 'Manage' })).toBeTruthy()
  })

  it('drops empty meta items so separators never double', () => {
    const { container } = render(
      <MetaRow items={['one', null, undefined, false, 'two']} />,
    )
    expect(container.querySelectorAll('li')).toHaveLength(2)
    expect(container.querySelectorAll('[aria-hidden="true"]')).toHaveLength(1)
  })

  it('renders nothing for an all-empty meta row', () => {
    const { container } = render(<MetaRow items={[null, false]} />)
    expect(container.firstChild).toBeNull()
  })
})

describe('ObjectCardSkeleton', () => {
  it('announces loading once and hides the placeholder shapes', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <ObjectCardSkeleton count={2} />
      </I18nextProvider>,
    )
    const status = screen.getByRole('status')
    expect(status.getAttribute('aria-label')).toBe(
      commonEn.cardSkeleton.loading,
    )
    expect(status.querySelectorAll('li')).toHaveLength(2)
  })
})
