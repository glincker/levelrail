import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ExportProjectDialog } from './ExportProjectDialog'
import envEditorEn from '../../locales/en/envEditor.json'
import projectExportEn from '../../locales/en/projectExport.json'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['projectExport', 'envEditor'],
  defaultNS: 'projectExport',
  resources: { en: { projectExport: projectExportEn, envEditor: envEditorEn } },
  interpolation: { escapeValue: false },
})

function jsonRes(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: new Headers(),
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function tarRes(): Response {
  return {
    ok: true,
    status: 200,
    headers: new Headers({
      'Content-Disposition': 'attachment; filename="project-shop-compose.tar"',
    }),
    blob: () => Promise.resolve(new Blob(['tar'])),
  } as unknown as Response
}

function setup(fetchImpl: (url: string, init?: RequestInit) => Response) {
  const urls: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = typeof input === 'string' ? input : (input as Request).url
      urls.push(url)
      return Promise.resolve(fetchImpl(url, init))
    }),
  )
  const createObjectURL = vi.fn(() => 'blob:x')
  Object.assign(URL, { createObjectURL, revokeObjectURL: vi.fn() })
  render(
    <QueryClientProvider client={createTestQueryClient()}>
      <I18nextProvider i18n={testI18n}>
        <ExportProjectDialog
          projectId="proj_1"
          projectName="shop"
          appNames={['web']}
        />
      </I18nextProvider>
    </QueryClientProvider>,
  )
  return { urls, createObjectURL, user: userEvent.setup() }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('ExportProjectDialog', () => {
  it('keeps secrets off by default and warns when turned on', async () => {
    const { user } = setup(() => tarRes())
    await user.click(screen.getByRole('button', { name: /Export/ }))
    const toggle = await screen.findByRole('switch', {
      name: 'Include secret values',
    })
    expect(toggle).not.toBeChecked()
    expect(screen.queryByText(/live secrets in plain text/)).toBeNull()
    await user.click(toggle)
    expect(screen.getByText(/live secrets in plain text/)).toBeTruthy()
  })

  it('downloads without include_secrets by default', async () => {
    const { user, urls, createObjectURL } = setup(() => tarRes())
    await user.click(screen.getByRole('button', { name: /Export/ }))
    await user.click(await screen.findByRole('button', { name: /Download/ }))
    await waitFor(() => {
      expect(createObjectURL).toHaveBeenCalled()
    })
    expect(urls).toEqual(['/api/v1/projects/proj_1/export'])
  })

  it('asks for step-up before secrets are released, then retries', async () => {
    let granted = false
    const { user, urls, createObjectURL } = setup((url, init) => {
      if (url === '/api/v1/auth/step-up' && init?.method === 'POST') {
        granted = true
        return jsonRes({})
      }
      if (url === '/api/v1/auth/step-up') {
        return jsonRes({
          methods: ['password'],
          code_required: false,
          policy: 'step_up',
          grant_ttl_seconds: 300,
          hide_after_seconds: 1,
        })
      }
      return granted
        ? tarRes()
        : jsonRes({ error: 'confirm', code: 'step_up_required' }, 403)
    })
    await user.click(screen.getByRole('button', { name: /Export/ }))
    await user.click(
      await screen.findByRole('switch', { name: 'Include secret values' }),
    )
    await user.click(screen.getByRole('button', { name: /Download/ }))
    expect(urls[0]).toBe('/api/v1/projects/proj_1/export?include_secrets=true')
    expect(createObjectURL).not.toHaveBeenCalled()
    await user.type(await screen.findByLabelText('Password'), 'pw')
    await user.click(screen.getByRole('button', { name: 'Confirm' }))
    await waitFor(() => {
      expect(createObjectURL).toHaveBeenCalled()
    })
  })

  it('shows the real error message', async () => {
    const { user } = setup(() =>
      jsonRes({ error: 'project has no apps to export' }, 404),
    )
    await user.click(screen.getByRole('button', { name: /Export/ }))
    await user.click(await screen.findByRole('button', { name: /Download/ }))
    expect(
      await screen.findByText('project has no apps to export'),
    ).toBeTruthy()
  })
})
