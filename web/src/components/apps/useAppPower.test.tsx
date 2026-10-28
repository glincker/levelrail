import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AppDetail } from '../../types/appDetail'
import { CommonI18n } from '../../test/commonI18n'
import { useAppPower } from './useAppPower'

function Harness({ app }: { app: AppDetail }) {
  const power = useAppPower(app)
  return (
    <>
      <button onClick={power.restart}>restart</button>
      <button onClick={power.toggleRunning}>toggle</button>
      {power.confirmDialog}
    </>
  )
}

function setup(app: Partial<AppDetail> = {}) {
  const spy = vi.spyOn(globalThis, 'fetch').mockResolvedValue({
    ok: true,
    status: 202,
    json: () => Promise.resolve({ name: 'web' }),
  } as unknown as Response)
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  render(
    <CommonI18n>
      <QueryClientProvider client={client}>
        <Harness app={{ name: 'web', suspended: false, ...app } as AppDetail} />
      </QueryClientProvider>
    </CommonI18n>,
  )
  return spy
}

describe('useAppPower confirm', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    window.localStorage.clear()
  })

  it('restart needs a confirm before the request', async () => {
    const spy = setup()
    await userEvent.click(screen.getByText('restart'))
    expect(spy).not.toHaveBeenCalled()
    await userEvent.click(screen.getByRole('button', { name: 'Restart' }))
    await waitFor(() => expect(spy).toHaveBeenCalled())
    expect(spy.mock.calls[0]?.[0]).toBe('/api/v1/apps/web/restart')
  })

  it('stop always confirms and never offers the opt-out', async () => {
    window.localStorage.setItem('app-power:skip-restart-confirm', '1')
    const spy = setup()
    await userEvent.click(screen.getByText('toggle'))
    expect(spy).not.toHaveBeenCalled()
    expect(screen.queryByRole('checkbox')).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Stop' }))
    await waitFor(() => expect(spy).toHaveBeenCalled())
    expect(spy.mock.calls[0]?.[0]).toBe('/api/v1/apps/web/stop')
  })

  it('restart opt-out is remembered and skips the next confirm', async () => {
    const spy = setup()
    await userEvent.click(screen.getByText('restart'))
    await userEvent.click(screen.getByRole('checkbox'))
    await userEvent.click(screen.getByRole('button', { name: 'Restart' }))
    await waitFor(() => expect(spy).toHaveBeenCalledTimes(1))
    await userEvent.click(screen.getByText('restart'))
    await waitFor(() => expect(spy).toHaveBeenCalledTimes(2))
    expect(screen.queryByRole('dialog')).toBeNull()
  })
})
