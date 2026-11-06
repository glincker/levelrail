import type { ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import getStartedEn from '../../locales/en/getStarted.json'
import { buildFirstRun, type FirstRunInput } from '../../lib/firstRunSteps'
import type { AppListEntry } from '../../types/appDetail'
import { FirstRunStrip } from './FirstRunStrip'

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    children,
    to,
    ...rest
  }: {
    children: ReactNode
    to: string
    'data-primary'?: boolean
  }) => (
    <a href={to} {...rest}>
      {children}
    </a>
  ),
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { getStarted: getStartedEn } },
  interpolation: { escapeValue: false },
})

function app(over: Partial<AppListEntry> = {}): AppListEntry {
  return { name: 'sample-app', domains: [], ...over } as AppListEntry
}

function renderStrip(input: Partial<FirstRunInput>, deploying = false) {
  const model = buildFirstRun({
    phase: 'none',
    sampleName: 'sample-app',
    ...input,
  })
  const { container } = render(
    <I18nextProvider i18n={i18n}>
      <FirstRunStrip model={model} onDeploy={vi.fn()} deploying={deploying} />
    </I18nextProvider>,
  )
  return container
}

describe('FirstRunStrip', () => {
  it('shows three steps and one primary deploy button with nothing deployed', () => {
    const c = renderStrip({})
    expect(c.querySelectorAll('li')).toHaveLength(3)
    expect(c.querySelectorAll('[data-primary]')).toHaveLength(1)
    expect(screen.getByRole('button', { name: 'Deploy sample app' })).toBe(
      c.querySelector('[data-primary]'),
    )
  })

  it('keeps the primary action first in focus order', () => {
    const c = renderStrip({})
    const focusable = c.querySelectorAll('a, button')
    expect(focusable[0]).toBe(c.querySelector('[data-primary]'))
  })

  it('disables the primary button while the app starts, without a domain yet', () => {
    const c = renderStrip({ app: app(), phase: 'deploying' })
    expect(
      screen.getByRole('button', { name: 'Deploy sample app' }),
    ).toBeDisabled()
    expect(
      c.querySelector('[data-step="deploy"]')?.getAttribute('data-state'),
    ).toBe('working')
    expect(
      c.querySelector('[data-step="domain"]')?.getAttribute('data-state'),
    ).toBe('todo')
  })

  it('marks the domain step working while the certificate is pending', () => {
    const c = renderStrip({
      app: app({ domains: ['demo.example.com'] }),
      phase: 'deploying',
      https: { state: 'pending', staging: false },
    })
    expect(
      c.querySelector('[data-step="domain"]')?.getAttribute('data-state'),
    ).toBe('done')
    const pending = renderStrip({
      app: app(),
      phase: 'deploying',
      https: { state: 'pending', staging: false, domain: 'demo.example.com' },
    })
    expect(
      pending.querySelector('[data-step="domain"]')?.getAttribute('data-state'),
    ).toBe('working')
  })

  it('collapses to one live line with an open link and add domain', () => {
    const c = renderStrip({
      app: app(),
      phase: 'live',
      address: 'http://203-0-113-9.sslip.io',
    })
    expect(c.querySelectorAll('li')).toHaveLength(0)
    expect(c.querySelectorAll('[data-primary]')).toHaveLength(1)
    const open = c.querySelector('a[data-primary]')
    expect(open?.getAttribute('href')).toBe('http://203-0-113-9.sslip.io')
    expect(screen.getByText('Add domain')).toBeInTheDocument()
  })

  it('points the primary action at the doctor when a check fails', () => {
    const c = renderStrip({
      doctor: {
        ok: false,
        checks: [
          { code: 'x', name: 'x', status: 'fail', message: 'Docker is down' },
        ],
      },
    })
    expect(c.querySelector('a[data-primary]')?.getAttribute('href')).toBe(
      '/inbox',
    )
    expect(screen.getByText('Docker is down')).toBeInTheDocument()
  })
})
