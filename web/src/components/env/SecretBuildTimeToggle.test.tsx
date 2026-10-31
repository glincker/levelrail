import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { QueryClientProvider } from '@tanstack/react-query'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import commonEn from '../../locales/en/common.json'
import secretRotationEn from '../../locales/en/secretRotation.json'
import { useSecretKeys } from '../../queries/secrets'
import { SecretBuildTimeToggle } from './SecretBuildTimeToggle'
import { createTestQueryClient } from '@/test/testQueryClient'

const toastAdd = vi.fn()
vi.mock('@/components/ui/toast', () => ({
  toast: { add: (t: unknown) => toastAdd(t) as unknown },
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['common', 'secretRotation'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, secretRotation: secretRotationEn } },
  interpolation: { escapeValue: false },
})

let buildTime = false
let postStatus = 204
const posted: { url: string; body: { build_time: boolean } }[] = []
const fetchMock = vi.fn()

function list() {
  return [
    {
      key: 'NPM_TOKEN',
      locked: false,
      updated_at: '2026-01-01T00:00:00Z',
      stale: false,
      build_time: buildTime,
    },
  ]
}

beforeEach(() => {
  buildTime = false
  postStatus = 204
  toastAdd.mockReset()
  fetchMock.mockReset()
  posted.length = 0
  fetchMock.mockImplementation((_url: string, init?: RequestInit) => {
    if (init?.method === 'POST') {
      if (postStatus === 204) {
        const body = JSON.parse(init.body as string) as {
          build_time: boolean
        }
        posted.push({ url: _url, body })
        buildTime = body.build_time
        return Promise.resolve(new Response(null, { status: 204 }))
      }
      return Promise.resolve(
        new Response(JSON.stringify({ error: 'boom' }), { status: postStatus }),
      )
    }
    return Promise.resolve(
      new Response(JSON.stringify(list()), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
  })
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

function Row({ canEdit }: { canEdit: boolean }) {
  const keys = useSecretKeys('web')
  const row = keys.data?.[0]
  if (!row) return null
  return (
    <SecretBuildTimeToggle
      appName="web"
      secretKey={row.key}
      enabled={row.buildTime}
      canEdit={canEdit}
    />
  )
}

function setup(canEdit = true) {
  const client = createTestQueryClient()
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <Row canEdit={canEdit} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

const posts = () => posted

describe('SecretBuildTimeToggle', () => {
  it('confirms before enabling, then posts and refreshes the list', async () => {
    setup()
    const sw = await screen.findByRole('switch', { name: 'Build NPM_TOKEN' })
    await userEvent.click(sw)
    expect(posts()).toHaveLength(0)
    expect(screen.getByText('Let builds read NPM_TOKEN?')).toBeInTheDocument()
    await userEvent.click(
      screen.getByRole('button', { name: 'Enable for builds' }),
    )
    await waitFor(() => {
      expect(posts()).toHaveLength(1)
    })
    expect(posts()[0]?.url).toBe(
      '/api/v1/apps/web/secrets/NPM_TOKEN/build-time',
    )
    expect(posts()[0]?.body).toEqual({ build_time: true })
    await waitFor(() => {
      expect(
        screen.getByRole('switch', { name: 'Build NPM_TOKEN' }),
      ).toBeChecked()
    })
  })

  it('disables without a confirm dialog', async () => {
    buildTime = true
    setup()
    const sw = await screen.findByRole('switch', { name: 'Build NPM_TOKEN' })
    expect(sw).toBeChecked()
    await userEvent.click(sw)
    expect(screen.queryByText('Let builds read NPM_TOKEN?')).toBeNull()
    await waitFor(() => {
      expect(posts()).toHaveLength(1)
    })
    expect(posts()[0]?.body).toEqual({ build_time: false })
  })

  it('rolls back and toasts when the call fails', async () => {
    buildTime = true
    postStatus = 500
    setup()
    const sw = await screen.findByRole('switch', { name: 'Build NPM_TOKEN' })
    await userEvent.click(sw)
    await waitFor(() => {
      expect(toastAdd).toHaveBeenCalledWith(
        expect.objectContaining({ type: 'error' }),
      )
    })
    await waitFor(() => {
      expect(
        screen.getByRole('switch', { name: 'Build NPM_TOKEN' }),
      ).toBeChecked()
    })
  })

  it('hides the control for read-only users', async () => {
    setup(false)
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled()
    })
    expect(screen.queryByRole('switch')).toBeNull()
    expect(screen.queryByText('Build')).toBeNull()
  })

  it('shows a read-only badge when enabled', async () => {
    buildTime = true
    setup(false)
    expect(await screen.findByText('Build')).toBeInTheDocument()
    expect(screen.queryByRole('switch')).toBeNull()
  })
})
