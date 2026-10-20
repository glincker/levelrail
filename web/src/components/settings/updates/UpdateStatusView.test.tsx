import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import updatesEn from '../../../locales/en/updates.json'
import { UpdateStatusView } from './UpdateStatusView'
import {
  deriveUpdateState,
  type UpdateState,
  type UpdateStateInput,
} from './updateState'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['updates'],
  defaultNS: 'updates',
  resources: { en: { updates: updatesEn } },
  interpolation: { escapeValue: false },
})

const base: UpdateStateInput = {
  latestVersion: 'v0.4.0',
  updateAvailable: false,
  hostBinaryEdge: false,
  scheduleActive: false,
  upgradeRunning: false,
  checkFailed: false,
}

describe('deriveUpdateState', () => {
  const cases: Array<[string, Partial<UpdateStateInput>, UpdateState]> = [
    ['up to date', {}, 'upToDate'],
    ['available', { updateAvailable: true }, 'available'],
    ['scheduled', { updateAvailable: true, scheduleActive: true }, 'scheduled'],
    [
      'in progress wins',
      { scheduleActive: true, upgradeRunning: true },
      'inProgress',
    ],
    [
      'check failed',
      { updateAvailable: true, checkFailed: true },
      'checkFailed',
    ],
    ['no releases', { latestVersion: null }, 'noReleases'],
    ['host binary edge', { hostBinaryEdge: true }, 'hostBinary'],
  ]
  it.each(cases)('%s', (_n, over, want) => {
    expect(deriveUpdateState({ ...base, ...over })).toBe(want)
  })
})

describe('UpdateStatusView', () => {
  function view(state: UpdateState, extra?: { releaseUrl?: string }) {
    render(
      <I18nextProvider i18n={testI18n}>
        <UpdateStatusView
          state={state}
          currentVersion="v0.3.0"
          latestVersion="v0.4.0"
          edge={false}
          lastChecked="2m ago"
          checking={false}
          errorMessage="boom"
          releaseUrl={extra?.releaseUrl ?? null}
          onCheck={() => undefined}
        />
      </I18nextProvider>,
    )
  }

  it('shows the pill, version and last checked time for an available update', () => {
    view('available', { releaseUrl: 'https://example.test/r' })
    expect(screen.getByText('Update available')).toBeTruthy()
    expect(screen.getByText('v0.3.0')).toBeTruthy()
    expect(screen.getByText('A new version is available: v0.4.0')).toBeTruthy()
    expect(screen.getByText('Last checked 2m ago')).toBeTruthy()
    expect(
      screen.getByRole('link', { name: /View release notes/ }),
    ).toBeTruthy()
  })

  it('explains a failed check with the error message', () => {
    view('checkFailed')
    expect(screen.getByRole('alert').textContent).toContain('boom')
    expect(screen.getByRole('button', { name: /Check now/ })).toBeTruthy()
  })

  it('hides the release link when up to date', () => {
    view('upToDate')
    expect(screen.getByText('Up to date')).toBeTruthy()
    expect(screen.queryByRole('link')).toBeNull()
  })
})
