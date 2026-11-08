import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { NewSurfacesProviders } from '../../test/NewSurfacesProviders'
import { jsonResponse } from '../../test/newSurfacesI18n'
import { DatabaseDataActionsCard } from './DatabaseDataActionsCard'

vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))

let granted: string[] = []
vi.mock('@/hooks/useAbilities', () => ({
  useAbilities: () => ({
    role: 'custom',
    can: (a: string) => granted.includes(a),
  }),
}))

const backup = {
  id: 'bk1',
  status: 'succeeded',
  started_at: '2026-11-01T00:00:00Z',
  resource_kind: 'database',
  target_id: 't1',
}

function stubApi(importEmpty: boolean) {
  const fetchMock = vi.fn((input: string, init?: RequestInit) => {
    if (input.endsWith('/backups') && init?.method === 'POST')
      return Promise.resolve(
        jsonResponse({ id: 'bk2', status: 'running' }, 202),
      )
    if (input.endsWith('/backup-targets'))
      return Promise.resolve(
        jsonResponse([
          { id: 't1', name: 'main' },
          { id: 't2', name: 'offsite' },
        ]),
      )
    if (input.endsWith('/backups'))
      return Promise.resolve(jsonResponse([backup]))
    if (input.endsWith('/restore-as-new') && init?.method === 'POST')
      return Promise.resolve(jsonResponse({ id: 'c1', status: 'running' }, 202))
    if (input.endsWith('/import'))
      return Promise.resolve(
        jsonResponse({
          database: 'orders',
          engine: 'postgres',
          empty: importEmpty,
        }),
      )
    return Promise.resolve(jsonResponse([]))
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

afterEach(() => {
  vi.unstubAllGlobals()
  granted = []
})

function renderCard() {
  return render(
    <NewSurfacesProviders>
      <DatabaseDataActionsCard database="orders" />
    </NewSurfacesProviders>,
  )
}

describe('DatabaseDataActionsCard', () => {
  it('renders all three actions and disables download without the ability', () => {
    stubApi(true)
    renderCard()
    expect(
      screen.getByRole('button', { name: /Download latest dump/ }),
    ).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Clone' })).toBeDisabled()
    expect(screen.getByLabelText('Dump file')).toBeDisabled()
    expect(screen.getByText(/read:sensitive/)).toBeInTheDocument()
  })

  it('backs up now to the chosen target and explains when not allowed', async () => {
    granted = ['write:sensitive']
    const fetchMock = stubApi(true)
    renderCard()
    const select = await screen.findByLabelText('Backup target')
    await waitFor(() => expect(select).toHaveValue('t1'))
    await userEvent.selectOptions(select, 't2')
    await userEvent.click(screen.getByRole('button', { name: 'Back up now' }))
    await waitFor(() => {
      const call = fetchMock.mock.calls.find(
        ([u, i]) => u.endsWith('/backups') && i?.method === 'POST',
      )
      expect(JSON.parse(call?.[1]?.body as string)).toEqual({ target_id: 't2' })
    })
  })

  it('disables back up now without the ability', () => {
    stubApi(true)
    renderCard()
    expect(screen.getByRole('button', { name: 'Back up now' })).toBeDisabled()
    expect(screen.getByText(/Backing up needs/)).toBeInTheDocument()
  })

  it('submits the clone with the prefilled name', async () => {
    granted = ['read:sensitive', 'write:sensitive']
    const fetchMock = stubApi(true)
    renderCard()
    expect(screen.getByLabelText('New database name')).toHaveValue(
      'orders-clone',
    )
    const button = screen.getByRole('button', { name: 'Clone' })
    await waitFor(() => expect(button).toBeEnabled())
    await userEvent.click(button)
    await waitFor(() => {
      const call = fetchMock.mock.calls.find(([u]) =>
        u.endsWith('/restore-as-new'),
      )
      expect(JSON.parse(call?.[1]?.body as string)).toEqual({
        backup_id: 'bk1',
        new_name: 'orders-clone',
      })
    })
  })

  it('needs the typed name before importing over existing data', async () => {
    granted = ['root']
    stubApi(false)
    renderCard()
    const file = new File(['select 1'], 'dump.sql')
    fireEvent.change(screen.getByLabelText('Dump file'), {
      target: { files: [file] },
    })
    const submit = await screen.findByRole('button', { name: 'Replace data' })
    expect(submit).toBeDisabled()
    await userEvent.type(
      screen.getByLabelText('Type orders to confirm'),
      'orders',
    )
    expect(submit).toBeEnabled()
  })
})
