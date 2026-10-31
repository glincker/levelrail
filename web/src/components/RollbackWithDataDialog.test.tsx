import { render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { RollbackWithDataDialog } from './RollbackWithDataDialog'
import deploysEn from '../locales/en/deploys.json'
import type { PredeploySnapshot } from '../types/predeploySnapshot'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@/components/ui/toast', () => ({
  toast: { add: vi.fn() },
}))

const instance = i18next.createInstance()

beforeAll(async () => {
  await instance.use(initReactI18next).init({
    lng: 'en',
    fallbackLng: 'en',
    ns: ['deploys'],
    defaultNS: 'deploys',
    resources: { en: { deploys: deploysEn } },
    interpolation: { escapeValue: false },
  })
})

const snap = (over: Partial<PredeploySnapshot> = {}): PredeploySnapshot => ({
  id: 's1',
  deploy_id: 'dep_1',
  prior_deploy_id: 'dep_0',
  database_name: 'orders-db',
  backup_id: 'b1',
  status: 'succeeded',
  size_bytes: 10,
  created_at: '2026-10-28T10:00:00Z',
  ...over,
})

function renderDialog(rows: PredeploySnapshot[], protectedEnv = false) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve(
        new Response(JSON.stringify(rows), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      ),
    ),
  )
  const client = createTestQueryClient()
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={instance}>
        <RollbackWithDataDialog
          appName="web"
          deployId="dep_1"
          protectedEnv={protectedEnv}
          open
          onOpenChange={vi.fn()}
        />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('RollbackWithDataDialog', () => {
  it('lists the snapshot, warns about the swap and enables the action', async () => {
    renderDialog([snap()])
    expect(await screen.findByText('orders-db')).toBeInTheDocument()
    expect(screen.getByText(/restored into a new database/)).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Roll back with data' }),
    ).toBeEnabled()
  })

  it('disables with a reason when no snapshot exists for the deploy', async () => {
    renderDialog([snap({ deploy_id: 'other' })])
    expect(
      await screen.findByText(/No usable snapshot was taken/),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Roll back with data' }),
    ).toBeDisabled()
  })

  it('disables with a reason in a protected environment', async () => {
    renderDialog([snap()], true)
    expect(await screen.findByText(/protected environment/)).toBeInTheDocument()
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Roll back with data' }),
      ).toBeDisabled(),
    )
  })
})
