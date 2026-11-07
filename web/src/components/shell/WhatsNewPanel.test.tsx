import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import commonEn from '../../locales/en/common.json'
import { WhatsNewPanel } from './WhatsNewPanel'

interface UpdatesState {
  data?: {
    current_version: string
    latest_version: string | null
    update_available: boolean
    release_url: string | null
  }
  isPending: boolean
  isError: boolean
}

const mocks = vi.hoisted(() => {
  const updates: UpdatesState = { isPending: false, isError: false }
  return { updates, bullets: [] as string[] }
})

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
}))
vi.mock('../../hooks/useIsRoot', () => ({ useIsRoot: () => true }))
vi.mock('@tanstack/react-query', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-query')>()),
  useQuery: (opts: { queryKey: readonly string[] }) =>
    opts.queryKey[0] === 'changelog'
      ? {
          data: {
            current_version: '0.8.0',
            entries: [
              { version: '0.9.2', date: '2026-11-01', bullets: mocks.bullets },
            ],
          },
          isError: false,
          isPending: false,
        }
      : mocks.updates,
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['common'],
  defaultNS: 'common',
  resources: { en: { common: commonEn } },
  interpolation: { escapeValue: false },
})

function open() {
  render(
    <I18nextProvider i18n={i18n}>
      <WhatsNewPanel />
    </I18nextProvider>,
  )
  fireEvent.click(screen.getByTitle("What's new"))
}

describe('WhatsNewPanel', () => {
  beforeEach(() => {
    mocks.bullets = ['apps: add a thing for the dashboard page']
    window.localStorage.clear()
  })

  it('says update available when behind', () => {
    mocks.updates = {
      data: {
        current_version: '0.8.0',
        latest_version: '0.9.2',
        update_available: true,
        release_url: null,
      },
      isPending: false,
      isError: false,
    }
    open()
    expect(screen.getByText(/Update available/)).toBeTruthy()
    expect(screen.queryByText('You are on the latest version')).toBeNull()
  })

  it('says latest only when the data says so', () => {
    mocks.updates = {
      data: {
        current_version: '0.9.2',
        latest_version: '0.9.2',
        update_available: false,
        release_url: null,
      },
      isPending: false,
      isError: false,
    }
    open()
    expect(screen.getByText('You are on the latest version')).toBeTruthy()
  })

  it('makes no claim when the request failed', () => {
    mocks.updates = { isPending: false, isError: true }
    open()
    expect(screen.getByText('Version status unknown')).toBeTruthy()
    expect(screen.queryByText('You are on the latest version')).toBeNull()
  })

  it('collapses a 203 bullet release into capped scope groups', () => {
    mocks.updates = {
      data: {
        current_version: '0.8.0',
        latest_version: '0.9.2',
        update_available: true,
        release_url: 'https://example.com/notes',
      },
      isPending: false,
      isError: false,
    }
    mocks.bullets = Array.from(
      { length: 203 },
      (_, i) => `scope${i % 12}: tweak number ${i} of something`,
    )
    open()
    expect(screen.queryByText(/All changes \(/)).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: /^All changes/ }))
    const groupButtons = screen
      .getAllByRole('button')
      .filter((b) =>
        b.getAttribute('aria-controls')?.startsWith('whatsnew-scope'),
      )
    expect(groupButtons.length).toBe(8)
    groupButtons.forEach((b) => {
      fireEvent.click(b)
    })
    expect(screen.getAllByRole('listitem').length).toBeLessThanOrEqual(200)
    expect(
      screen.getByRole('link', { name: /Open the full release notes/ }),
    ).toBeTruthy()
  })
})
