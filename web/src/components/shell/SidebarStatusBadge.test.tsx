import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import commonEn from '../../locales/en/common.json'
import type { ShellStatus } from '../../lib/shellStatus'
import {
  SidebarStatusBadge,
  SidebarStatusBadgeView,
} from './SidebarStatusBadge'

const shell = vi.hoisted(() => ({ status: 'healthy' }))
vi.mock('./useShellStatus', () => ({ useShellStatus: () => shell.status }))

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

describe('SidebarStatusBadge', () => {
  beforeEach(() => {
    shell.status = 'healthy'
  })

  it.each(['incident', 'degraded', 'unknown'])(
    'keeps showing %s instead of hiding it',
    (status) => {
      shell.status = status
      render(
        <I18nextProvider i18n={i18n}>
          <SidebarStatusBadge />
        </I18nextProvider>,
      )
      expect(screen.getByRole('img').getAttribute('data-status')).toBe(status)
    },
  )

  it('treats an available update as healthy', () => {
    shell.status = 'update'
    render(
      <I18nextProvider i18n={i18n}>
        <SidebarStatusBadge />
      </I18nextProvider>,
    )
    expect(screen.getByRole('img').getAttribute('data-status')).toBe('healthy')
  })
})
