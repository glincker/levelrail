import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/react'
import { EnvironmentsTable } from './EnvironmentsTable'
import { env, renderWithProviders } from './environmentsTestUtils'

afterEach(() => {
  vi.unstubAllGlobals()
})

const rows = [
  env({ id: 'env_dev', name: 'Development', kind: 'dev', app_count: 2 }),
  env({
    id: 'env_production',
    name: 'Production',
    kind: 'production',
    protected: true,
    app_count: 3,
    database_count: 1,
  }),
  env({ id: 'env_custom', name: 'Sandbox', kind: 'custom' }),
]

describe('EnvironmentsTable', () => {
  it('shows kind badges, counts and the built-in marker', () => {
    renderWithProviders(<EnvironmentsTable environments={rows} />)
    expect(
      screen.getByText('Production', { selector: '[data-slot="badge"]' }),
    ).toBeInTheDocument()
    expect(screen.getAllByText('Built in')).toHaveLength(2)
    expect(screen.getByText('3')).toBeInTheDocument()
  })

  it('does not let a built-in environment be deleted, but a custom one can be', () => {
    renderWithProviders(<EnvironmentsTable environments={rows} />)
    expect(screen.getByLabelText('Delete Development')).toBeDisabled()
    expect(screen.getByLabelText('Delete Production')).toBeDisabled()
    expect(screen.getByLabelText('Delete Sandbox')).toBeEnabled()
  })

  it('toggles protection through PATCH', async () => {
    const fetchMock = vi.fn(() =>
      Promise.resolve(
        new Response(
          JSON.stringify(env({ id: 'env_custom', protected: true })),
          {
            status: 200,
          },
        ),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    renderWithProviders(<EnvironmentsTable environments={rows} />)
    fireEvent.click(screen.getByRole('switch', { name: 'Protected, Sandbox' }))
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled()
    })
    const [url, init] = fetchMock.mock.calls[0] as unknown as [
      string,
      RequestInit,
    ]
    expect(url).toBe('/api/v1/environments/env_custom')
    expect(init.method).toBe('PATCH')
    expect(JSON.parse(init.body as string)).toEqual({ protected: true })
  })

  it('opens the delete dialog and asks where tagged items go', () => {
    renderWithProviders(<EnvironmentsTable environments={rows} />)
    const inUse = [
      ...rows.slice(0, 1),
      env({
        id: 'env_custom',
        name: 'Sandbox',
        app_count: 1,
        database_count: 2,
      }),
    ]
    renderWithProviders(<EnvironmentsTable environments={inUse} />)
    fireEvent.click(
      screen.getAllByLabelText('Delete Sandbox').at(-1) as HTMLElement,
    )
    expect(
      screen.getByText(/1 app\(s\) and 2 database\(s\) are tagged/),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Delete environment' }),
    ).toBeDisabled()
  })
})
