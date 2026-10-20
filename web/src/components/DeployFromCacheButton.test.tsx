import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { toast } from '@/components/ui/toast'
import { DeployFromCacheButton } from './DeployFromCacheButton'
import gitBuildEn from '../locales/en/gitBuild.json'

vi.mock('@/components/ui/toast', () => ({
  toast: { add: vi.fn() },
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['gitBuild'],
  defaultNS: 'gitBuild',
  resources: { en: { gitBuild: gitBuildEn } },
  interpolation: { escapeValue: false },
})

function respond(status: number, body: unknown) {
  return vi.fn(() =>
    Promise.resolve(
      new Response(JSON.stringify(body), {
        status,
        headers: { 'Content-Type': 'application/json' },
      }),
    ),
  )
}

function renderButton() {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <DeployFromCacheButton appName="web" />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

async function clickDeploy() {
  await userEvent.click(
    screen.getByRole('button', { name: 'Deploy from cache' }),
  )
  await waitFor(() => expect(toast.add).toHaveBeenCalled())
  const [first] = vi.mocked(toast.add).mock.calls
  return first?.[0]
}

describe('DeployFromCacheButton', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.mocked(toast.add).mockClear()
  })

  it('posts to the from-cache route and reports success', async () => {
    const fetchMock = respond(202, { id: 'dep_1' })
    vi.stubGlobal('fetch', fetchMock)
    renderButton()
    expect(await clickDeploy()).toMatchObject({ type: 'success' })
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/apps/web/deploys/from-cache',
      expect.objectContaining({ method: 'POST' }),
    )
  })

  it('shows the server reason when the deploy is refused', async () => {
    vi.stubGlobal('fetch', respond(422, { error: 'no earlier deploy' }))
    renderButton()
    expect(await clickDeploy()).toMatchObject({
      type: 'error',
      description: 'no earlier deploy',
    })
  })
})
