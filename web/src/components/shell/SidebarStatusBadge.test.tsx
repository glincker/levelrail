import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import commonEn from '../../locales/en/common.json'
import type { ShellStatus } from '../../lib/shellStatus'
import { SidebarStatusBadgeView } from './SidebarStatusBadge'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['common'],
  defaultNS: 'common',
  resources: { en: { common: commonEn } },
  interpolation: { escapeValue: false },
})

const statuses: ShellStatus[] = [
  'healthy',
  'update',
  'degraded',
  'incident',
  'busy',
  'unknown',
]

describe('SidebarStatusBadgeView', () => {
  it.each(statuses)('labels %s with text, not color alone', (status) => {
    render(
      <I18nextProvider i18n={i18n}>
        <SidebarStatusBadgeView status={status} />
      </I18nextProvider>,
    )
    const badge = screen.getByRole('img')
    expect(badge.getAttribute('aria-label')).toBe(
      commonEn.shell.logoStatus[status],
    )
    expect(badge.getAttribute('title')).toBe(commonEn.shell.logoStatus[status])
    expect(badge.querySelector('svg')).not.toBeNull()
  })
})
