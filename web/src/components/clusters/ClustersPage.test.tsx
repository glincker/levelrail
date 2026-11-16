import type { ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import clustersEn from '../../locales/en/clusters.json'
import commonEn from '../../locales/en/common.json'
import { suggestClusterName } from '../../lib/clusterName'
import { ConnectClusterForm } from './ConnectClusterForm'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'clusters'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, clusters: clustersEn } },
  interpolation: { escapeValue: false },
})

function wrap(node: ReactNode) {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <I18nextProvider i18n={i18n}>{node}</I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('ConnectClusterForm', () => {
  it('suggests a name from the server host', () => {
    expect(suggestClusterName('https://203.0.113.10:6443')).toBe('203-0-113-10')
    expect(suggestClusterName('nonsense')).toBe('')
  })

  it('posts the kubeconfig once and never echoes it afterwards', async () => {
    const user = userEvent.setup()
    const fetchMock = vi.fn(() =>
      Promise.resolve(
        new Response(JSON.stringify({ name: 'lab', server: 'https://x' }), {
          status: 201,
          headers: { 'Content-Type': 'application/json' },
        }),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    wrap(<ConnectClusterForm existing={[]} />)

    await user.click(screen.getByLabelText('Kubeconfig', { selector: 'input' }))
    const box = await screen.findByLabelText('Kubeconfig', {
      selector: 'textarea',
    })
    await user.type(screen.getByLabelText('Name'), 'lab')
    await user.click(box)
    await user.paste('apiVersion: v1')
    await user.click(screen.getByRole('button', { name: 'Connect' }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled()
    })
    const calls = fetchMock.mock.calls as unknown as [string, RequestInit][]
    const body = JSON.parse(calls[0]?.[1].body as string) as {
      name: string
      kubeconfig: string
    }
    expect(body).toMatchObject({ name: 'lab', kubeconfig: 'apiVersion: v1' })
    await waitFor(() => {
      expect(box).toHaveValue('')
    })
    expect(screen.queryByText('apiVersion: v1')).not.toBeInTheDocument()
  })
})
