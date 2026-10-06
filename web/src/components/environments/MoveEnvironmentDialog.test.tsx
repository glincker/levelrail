import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/react'
import { MoveEnvironmentDialog } from './MoveEnvironmentDialog'
import { env, renderWithProviders } from './environmentsTestUtils'

afterEach(() => {
  vi.unstubAllGlobals()
})

const environments = [
  env({ id: 'env_dev', name: 'Development', kind: 'dev' }),
  env({
    id: 'env_production',
    name: 'Production',
    kind: 'production',
    protected: true,
  }),
]

function stubFetch(moveBody: unknown) {
  const fetchMock = vi.fn((_url: string, init?: RequestInit) => {
    if (init?.method === 'PUT') {
      return Promise.resolve(
        new Response(JSON.stringify(moveBody), { status: 200 }),
      )
    }
    return Promise.resolve(
      new Response(JSON.stringify(environments), { status: 200 }),
    )
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

async function pick(label: string) {
  const trigger = document.body.querySelector('#move-environment-target')
  if (!trigger) throw new Error('no target trigger')
  for (let attempt = 1; attempt <= 5; attempt++) {
    fireEvent.click(trigger)
    try {
      const option = screen.getByText(label)
      fireEvent.pointerDown(option, { pointerType: 'mouse' })
      fireEvent.click(option)
      return
    } catch (err) {
      if (attempt === 5) throw err
      await new Promise((r) => setTimeout(r, 20))
    }
  }
}

async function openDialog() {
  fireEvent.click(screen.getByRole('button', { name: /Change/ }))
  await screen.findByText('Environment')
}

describe('MoveEnvironmentDialog', () => {
  it('moves into an unprotected environment without any approval notice', async () => {
    const fetchMock = stubFetch({ name: 'web' })
    renderWithProviders(<MoveEnvironmentDialog kind="apps" name="web" />)
    await openDialog()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled()
    })
    await pick('Development')
    expect(screen.queryByText(/pending approval/)).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => {
      expect(fetchMock.mock.calls.some((c) => c[1]?.method === 'PUT')).toBe(
        true,
      )
    })
    const put = fetchMock.mock.calls.find((c) => c[1]?.method === 'PUT')
    expect(put?.[0]).toBe('/api/v1/apps/web/environment')
    expect(JSON.parse(put?.[1]?.body as string)).toEqual({
      environment_id: 'env_dev',
      confirm: false,
    })
  })

  it('explains protected approval and blocks until acknowledged, then sends confirm', async () => {
    const fetchMock = stubFetch({ pending_approval: { id: 'dap_1' } })
    renderWithProviders(<MoveEnvironmentDialog kind="databases" name="pg" />)
    await openDialog()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled()
    })
    await pick('Production')
    expect(
      await screen.findByText(/another person must approve/),
    ).toBeInTheDocument()
    const request = screen.getByRole('button', { name: 'Request move' })
    expect(request).toBeDisabled()
    fireEvent.click(screen.getByRole('checkbox'))
    expect(request).toBeEnabled()
    fireEvent.click(request)
    await waitFor(() => {
      expect(fetchMock.mock.calls.some((c) => c[1]?.method === 'PUT')).toBe(
        true,
      )
    })
    const put = fetchMock.mock.calls.find((c) => c[1]?.method === 'PUT')
    expect(put?.[0]).toBe('/api/v1/databases/pg/environment')
    expect(JSON.parse(put?.[1]?.body as string)).toEqual({
      environment_id: 'env_production',
      confirm: true,
    })
  })
})
