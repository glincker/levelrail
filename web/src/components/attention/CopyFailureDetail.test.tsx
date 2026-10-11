import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import attentionEn from '../../locales/en/attention.json'
import { CopyFailureDetail } from './CopyFailureDetail'
import { CopyItemControls } from './CopyItemControls'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['attention'],
  defaultNS: 'attention',
  resources: { en: { attention: attentionEn } },
  interpolation: { escapeValue: false },
})

const dismissMutate = vi.fn()
const snoozeNow = vi.fn()

vi.mock('../../queries/deviceAuth', () => ({
  useDismissAttentionItem: () => ({
    mutate: dismissMutate,
    isPending: false,
  }),
}))
vi.mock('../shell/snoozeStore', () => ({
  snoozeNow: (id: string, d: string) => {
    snoozeNow(id, d)
  },
}))
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({
      children,
      to,
      ...rest
    }: {
      children?: ReactNode
      to?: string
    } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
      <a href={to} {...rest}>
        {children}
      </a>
    ),
  }
})

describe('CopyFailureDetail', () => {
  it('shows the hint and keeps the raw error behind Technical details', () => {
    const { container } = render(
      <I18nextProvider i18n={testI18n}>
        <CopyFailureDetail
          hint="Check that it is reachable from this server."
          technical="docker: exec [sh -c ...]: timeout expired"
        />
      </I18nextProvider>,
    )
    expect(
      screen.getByText('Check that it is reachable from this server.'),
    ).toBeInTheDocument()
    const details = container.querySelector('details')
    expect(details).not.toBeNull()
    expect(details?.open).toBe(false)
    expect(screen.getByText('Technical details')).toBeInTheDocument()
  })

  it('renders nothing without a hint or technical text', () => {
    const { container } = render(
      <I18nextProvider i18n={testI18n}>
        <CopyFailureDetail />
      </I18nextProvider>,
    )
    expect(container).toBeEmptyDOMElement()
  })
})

describe('CopyItemControls', () => {
  it('snoozes and dismisses by the attempt id', () => {
    render(
      <I18nextProvider i18n={testI18n}>
        <CopyItemControls
          itemId="data_copy:geokit:1760000000"
          dismissKey="data_copy:geokit:1760000000"
          to="/databases/geokit"
        />
      </I18nextProvider>,
    )
    expect(screen.getByText('Open the database')).toHaveAttribute(
      'href',
      '/databases/geokit',
    )
    fireEvent.click(screen.getByLabelText('Snooze for a day'))
    expect(snoozeNow).toHaveBeenCalledWith('data_copy:geokit:1760000000', '1d')
    fireEvent.click(screen.getByLabelText('Dismiss'))
    expect(dismissMutate).toHaveBeenCalledWith(
      'data_copy:geokit:1760000000',
      expect.any(Object),
    )
  })
})
