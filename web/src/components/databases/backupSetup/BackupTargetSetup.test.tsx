import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import en from '../../../locales/en/dbBackupSetup.json'
import { createTestQueryClient } from '@/test/testQueryClient'
import { BackupTargetSetup } from './BackupTargetSetup'
import {
  buildTargetRequest,
  EMPTY_VALUES,
  PRESETS,
} from './backupTargetPresets'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { dbBackupSetup: en } },
  defaultNS: 'dbBackupSetup',
  interpolation: { escapeValue: false },
})

const json = (body: unknown, status = 200) =>
  Promise.resolve(
    new Response(JSON.stringify(body), {
      status,
      headers: { 'Content-Type': 'application/json' },
    }),
  )

function renderSetup(onCreated = vi.fn()) {
  render(
    <QueryClientProvider client={createTestQueryClient()}>
      <I18nextProvider i18n={i18n}>
        <BackupTargetSetup onCreated={onCreated} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
  return onCreated
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('backup target presets', () => {
  it('builds the R2 endpoint from the account id', () => {
    const r2 = PRESETS.find((p) => p.id === 'r2')
    expect(r2).toBeDefined()
    const req = buildTargetRequest(r2!, 'r2-backups', {
      ...EMPTY_VALUES,
      accountId: 'abc123',
      bucket: 'bk',
      accessKey: 'k',
      secret: 's',
    })
    expect(req).toMatchObject({
      provider: 'r2',
      endpoint: 'https://abc123.r2.cloudflarestorage.com',
      region: 'auto',
    })
  })
})

describe('BackupTargetSetup', () => {
  it('goes from the gallery to the R2 form and reports a failed test', async () => {
    const calls: string[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        calls.push(url)
        if (url.endsWith('/test')) return json({ error: 'access denied' }, 502)
        return json({
          id: 't1',
          name: 'r2-backups',
          provider: 'r2',
          bucket: 'bk',
        })
      }),
    )
    renderSetup()
    await userEvent.click(screen.getByRole('button', { name: 'Cloudflare R2' }))
    expect(screen.queryByLabelText('Region')).toBeNull()
    await userEvent.type(screen.getByLabelText('Account ID'), 'abc123')
    await userEvent.type(screen.getByLabelText('Bucket'), 'bk')
    await userEvent.type(screen.getByLabelText('Access key ID'), 'k')
    await userEvent.type(screen.getByLabelText('Secret access key'), 's')
    await userEvent.click(
      screen.getByRole('button', { name: 'Create and test' }),
    )
    await waitFor(() => {
      expect(screen.getByText(/connection test failed/)).toBeInTheDocument()
    })
    expect(calls.some((c) => c.endsWith('/t1/test'))).toBe(true)
    expect(
      screen.getByRole('button', { name: 'Remove and edit' }),
    ).toBeInTheDocument()
  })

  it('continues after a successful test', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) =>
        url.endsWith('/test')
          ? Promise.resolve(new Response(null, { status: 204 }))
          : json({
              id: 't1',
              name: 'local-backups',
              provider: 'local',
              bucket: 'backups',
            }),
      ),
    )
    const onCreated = renderSetup()
    await userEvent.click(screen.getByRole('button', { name: 'Local disk' }))
    await userEvent.click(
      screen.getByRole('button', { name: 'Create and test' }),
    )
    await userEvent.click(
      await screen.findByRole('button', { name: 'Continue' }),
    )
    expect(onCreated).toHaveBeenCalled()
  })
})
