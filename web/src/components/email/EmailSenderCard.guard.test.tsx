import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  Link,
  RouterProvider,
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
} from '@tanstack/react-router'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { CommonI18n } from '@/test/commonI18n'
import type { EmailSettings } from '../../queries/emailSettings'
import settingsEn from '../../locales/en/settings.json'
import commonEn from '../../locales/en/common.json'
import { EmailSenderCard } from './EmailSenderCard'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['settings', 'common'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn, common: commonEn } },
  interpolation: { escapeValue: false },
})

const settings: EmailSettings = { backend: '' }

function ok(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

async function mount() {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) =>
      Promise.resolve(ok(url.endsWith('/providers') ? { providers: [] } : {})),
    ),
  )
  const root = createRootRoute()
  const index = createRoute({
    getParentRoute: () => root,
    path: '/',
    component: () => (
      <div>
        <EmailSenderCard settings={settings} />
        <Link to="/login">leave</Link>
      </div>
    ),
  })
  const other = createRoute({
    getParentRoute: () => root,
    path: '/login',
    component: () => <p>other page</p>,
  })
  const router = createRouter({
    routeTree: root.addChildren([index, other]),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <CommonI18n>
      <I18nextProvider i18n={testI18n}>
        <QueryClientProvider client={queryClient}>
          <RouterProvider router={router} />
        </QueryClientProvider>
      </I18nextProvider>
    </CommonI18n>,
  )
  await screen.findByText('leave')
}

afterEach(() => vi.unstubAllGlobals())

describe('EmailSenderCard unsaved guard', () => {
  it('guards an edit made after a successful save', async () => {
    const user = userEvent.setup()
    await mount()
    await user.type(await screen.findByLabelText('From name'), 'Acme')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => {
      expect(fetch).toHaveBeenCalledWith(
        expect.stringContaining('/settings/email'),
        expect.objectContaining({ method: 'PUT' }),
      )
    })

    // Saved state is clean: leaving is not blocked until the next edit.
    await user.type(screen.getByLabelText('From name'), 'X')
    fireEvent.click(screen.getByText('leave'))
    expect(
      await screen.findByText('Discard unsaved changes?'),
    ).toBeInTheDocument()
    expect(screen.queryByText('other page')).not.toBeInTheDocument()
  })
})
