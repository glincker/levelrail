import { render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { PendingChangesCard } from './PendingChangesCard'
import smartDeployEn from '../../locales/en/smartDeploy.json'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['smartDeploy'],
  defaultNS: 'smartDeploy',
  resources: { en: { smartDeploy: smartDeployEn } },
  interpolation: { escapeValue: false },
})

function stub(status: number, body: unknown) {
  const fetchMock = vi.fn(() =>
    Promise.resolve(
      new Response(JSON.stringify(body), {
        status,
        headers: { 'Content-Type': 'application/json' },
      }),
    ),
  )
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function renderCard(image?: string) {
  const client = createTestQueryClient()
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <PendingChangesCard appName="web" image={image} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

const compare = {
  service_name: 'web',
  from: { is_current: false, image: 'web:1' },
  to: { is_current: true, image: 'web:2' },
  unsnapshotted_fields: [],
  note: '',
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('PendingChangesCard', () => {
  it('lists config changes and env keys without values', async () => {
    stub(200, {
      ...compare,
      changes: [{ field: 'image', from: 'web:1', to: 'web:2' }],
      env_changes: [{ key: 'API_KEY', kind: 'secret', status: 'added' }],
    })
    renderCard()
    expect(await screen.findByText('Image')).toBeInTheDocument()
    expect(screen.getByText('web:1 > web:2')).toBeInTheDocument()
    expect(screen.getByText('API_KEY')).toBeInTheDocument()
    expect(screen.getByText('Env added')).toBeInTheDocument()
  })

  it('says nothing differs when the diff is empty', async () => {
    stub(200, { ...compare, changes: [] })
    renderCard()
    expect(
      await screen.findByText('Nothing differs from the live deploy.'),
    ).toBeInTheDocument()
  })

  it('renders nothing when there is no earlier successful deploy', async () => {
    const fetchMock = stub(404, { error: 'no earlier successful deploy' })
    renderCard()
    await vi.waitFor(() => {
      expect(fetchMock).toHaveBeenCalled()
    })
    expect(screen.queryByRole('region')).not.toBeInTheDocument()
  })

  it('asks the server to diff against the typed image', async () => {
    const fetchMock = stub(200, { ...compare, changes: [] })
    renderCard('web:9')
    await screen
      .findByText('Nothing differs from the live deploy.')
      .catch(() => null)
    await vi.waitFor(() => {
      const urls = (fetchMock.mock.calls as unknown as string[][]).map(
        (c) => c[0] ?? '',
      )
      expect(urls.some((u) => u.includes('image=web%3A9'))).toBe(true)
    })
  })
})
