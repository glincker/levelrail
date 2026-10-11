import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import type { AttentionItem } from '../../lib/attention'
import commonEn from '../../locales/en/common.json'
import dashboardEn from '../../locales/en/dashboard.json'
import { resetSnoozeStoreForTests, snoozeNow } from '../shell/snoozeStore'
import { StatusHeader } from './StatusHeader'

const attention = vi.hoisted(() => ({ items: [] as AttentionItem[] }))

vi.mock('../../queries/attention', () => ({
  useAttentionItems: () => ({ items: attention.items, isLoading: false }),
}))
vi.mock('../../lib/connection', () => ({
  isInsecureRemoteConnection: () => false,
}))
vi.mock('../CreateResourceWizard', () => ({
  CreateResourceWizard: ({ trigger }: { trigger: React.ReactNode }) => trigger,
}))
vi.mock('./SetupChip', () => ({ SetupChip: () => null }))
vi.mock('../HelpLink', () => ({ HelpLink: () => null }))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['common', 'dashboard'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, dashboard: dashboardEn } },
  interpolation: { escapeValue: false },
})

const item = (
  id: string,
  severity: AttentionItem['severity'],
): AttentionItem => ({
  id,
  severity,
  title: id,
  detail: '',
  target: { kind: 'system' },
})

beforeEach(() => {
  resetSnoozeStoreForTests()
})

describe('StatusHeader', () => {
  it('uses the same count and label as the header chip', () => {
    attention.items = [
      item('a', 'critical'),
      item('b', 'warning'),
      item('c', 'warning'),
    ]
    render(
      <I18nextProvider i18n={i18n}>
        <StatusHeader />
      </I18nextProvider>,
    )
    expect(
      screen.getByRole('heading', { level: 1, name: 'Dashboard' }),
    ).toBeInTheDocument()
    expect(screen.getByText('3 incidents')).toBeInTheDocument()
  })

  it('does not count recommendations', () => {
    attention.items = [
      { ...item('dr', 'warning'), class: 'recommendation' },
      { ...item('guard', 'warning'), class: 'recommendation' },
    ]
    render(
      <I18nextProvider i18n={i18n}>
        <StatusHeader />
      </I18nextProvider>,
    )
    expect(screen.getByText('All systems normal')).toBeInTheDocument()
  })

  it('drops snoozed items from the count', () => {
    attention.items = [item('a', 'critical'), item('b', 'warning')]
    snoozeNow('a', '1h')
    render(
      <I18nextProvider i18n={i18n}>
        <StatusHeader />
      </I18nextProvider>,
    )
    expect(screen.getByText('1 incident')).toBeInTheDocument()
  })
})
