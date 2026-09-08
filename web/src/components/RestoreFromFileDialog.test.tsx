import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { RestoreFromFileDialog } from './RestoreFromFileDialog'

function renderDialog() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <RestoreFromFileDialog databaseName="mydb" />
    </QueryClientProvider>,
  )
}

describe('RestoreFromFileDialog', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('keeps the confirm button disabled until a file is chosen and the name is typed exactly', async () => {
    const user = userEvent.setup()
    renderDialog()

    await user.click(
      screen.getByRole('button', { name: /restore from a file/i }),
    )

    const confirmButton = screen.getByRole('button', {
      name: /restore database/i,
    })
    expect(confirmButton).toBeDisabled()

    await user.type(
      screen.getByLabelText(/type .mydb. to confirm/i),
      'mydb',
    )
    expect(confirmButton).toBeDisabled()

    const file = new File(['dump bytes'], 'dump.sql', {
      type: 'application/sql',
    })
    await user.upload(screen.getByLabelText(/dump file/i), file)
    expect(confirmButton).toBeEnabled()
  })

  it('uploads the chosen file to the restore-upload endpoint on confirm', async () => {
    const user = userEvent.setup()
    const fetchMock = vi.fn<(input: RequestInfo | URL, init?: RequestInit) => Promise<Response>>(
      () =>
        Promise.resolve({
          ok: true,
          status: 200,
          json: () =>
            Promise.resolve({
              id: 'rsh_1',
              database_name: 'mydb',
              status: 'succeeded',
            }),
        } as unknown as Response),
    )
    vi.stubGlobal('fetch', fetchMock)

    renderDialog()
    await user.click(
      screen.getByRole('button', { name: /restore from a file/i }),
    )
    await user.upload(
      screen.getByLabelText(/dump file/i),
      new File(['dump bytes'], 'dump.sql', { type: 'application/sql' }),
    )
    await user.type(
      screen.getByLabelText(/type .mydb. to confirm/i),
      'mydb',
    )
    await user.click(screen.getByRole('button', { name: /restore database/i }))

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/databases/mydb/restore-upload',
      expect.objectContaining({ method: 'POST' }),
    )
  })
})
