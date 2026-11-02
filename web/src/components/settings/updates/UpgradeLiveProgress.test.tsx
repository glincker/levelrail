import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import updatesEn from '../../../locales/en/updates.json'
import { deriveActions } from '@/lib/upgradeControl'
import {
  UpgradeLiveProgress,
  type UpgradeLiveProgressProps,
} from './UpgradeLiveProgress'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['updates'],
  defaultNS: 'updates',
  resources: { en: { updates: updatesEn } },
  interpolation: { escapeValue: false },
})

const props: UpgradeLiveProgressProps = {
  step: 'download',
  startedAtMs: 0,
  lastProgressMs: 0,
  now: 90_000,
  stalled: false,
  reconnecting: false,
  actions: deriveActions({
    running: true,
    stalled: false,
    failed: false,
    updateAvailable: true,
    canApply: true,
    step: 'download',
  }),
  busy: false,
  onCancel: vi.fn(),
  onRetry: vi.fn(),
  onRestart: vi.fn(),
  onRetryNow: vi.fn(),
}

function view(over: Partial<UpgradeLiveProgressProps>) {
  return render(
    <I18nextProvider i18n={i18n}>
      <UpgradeLiveProgress {...props} {...over} />
    </I18nextProvider>,
  )
}

describe('UpgradeLiveProgress', () => {
  it('shows bytes and percent when the total is known', () => {
    view({ downloadedBytes: 512 * 1024, totalBytes: 1024 * 1024 })
    expect(screen.getByText(/512.0 KB of 1.0 MB \(50%\)/)).toBeTruthy()
  })

  it('is indeterminate without a total', () => {
    view({})
    expect(screen.getByText('Size not reported yet')).toBeTruthy()
  })

  it('swaps the spinner for the stuck callout with all three actions', () => {
    view({
      stalled: true,
      actions: deriveActions({
        running: true,
        stalled: true,
        failed: false,
        updateAvailable: true,
        canApply: true,
        step: 'download',
      }),
    })
    expect(screen.getByRole('alert').textContent).toContain('looks stuck')
    expect(screen.getByRole('button', { name: 'Cancel update' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Restart server' })).toBeTruthy()
  })

  it('disables Cancel and says Cancelling when cancel was requested', () => {
    view({
      stalled: true,
      actions: deriveActions({
        running: true,
        stalled: true,
        failed: false,
        updateAvailable: true,
        canApply: true,
        step: 'download',
        cancelRequested: true,
      }),
    })
    const btn = screen.getByRole('button', { name: 'Cancelling...' })
    expect((btn as HTMLButtonElement).disabled).toBe(true)
  })

  it('offers Retry now when status requests keep failing', () => {
    view({ reconnecting: true })
    expect(screen.getByRole('button', { name: 'Retry now' })).toBeTruthy()
  })
})
