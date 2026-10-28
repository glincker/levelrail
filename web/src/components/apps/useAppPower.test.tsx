import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  afterEach,
  beforeEach,
  type MockInstance,
  describe,
  expect,
  it,
  vi,
} from 'vitest'
import type { AppDetail } from '../../types/appDetail'
import { CommonI18n } from '../../test/commonI18n'
import { POWER_TIMING, resetPowerTransitions } from './appPower'
import { useAppPower } from './useAppPower'

function urlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  return input instanceof URL ? input.href : input.url
}

const toastAdd = vi.hoisted(() => vi.fn())
vi.mock('@/components/ui/toast', async (orig) => ({
  ...(await orig<typeof import('@/components/ui/toast')>()),
  toast: { add: toastAdd },
}))
vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))
vi.mock('./appPower', async (orig) => ({
  ...(await orig<typeof import('./appPower')>()),
  POWER_TIMING: { graceMs: 100, timeoutMs: 60000, pollMs: 50, failPolls: 3 },
}))

function Harness({ app }: { app: AppDetail }) {
  const power = useAppPower(app)
  return (
    <>
      <button disabled={power.restarting} onClick={power.restart}>
        {power.restartLabel === 'Restart' ? 'restart' : power.restartLabel}
      </button>
      <button onClick={power.toggleRunning}>toggle</button>
      {power.confirmDialog}
    </>
  )
}

function posts(spy: MockInstance<typeof fetch>) {
  return spy.mock.calls.filter(([, init]) => init?.method === 'POST')
}

function setup(app: Partial<AppDetail> = {}) {
  const spy = vi.spyOn(globalThis, 'fetch').mockImplementation((input) =>
    Promise.resolve({
      ok: true,
      status: 202,
      json: () =>
        Promise.resolve(
          urlOf(input).endsWith('/deploys') ? [] : { name: 'web' },
        ),
    } as unknown as Response),
  )
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
    resetPowerTransitions()
  })

  it('restart needs a confirm before the request', async () => {
    const spy = setup()
    await userEvent.click(screen.getByText('restart'))
    expect(posts(spy)).toHaveLength(0)
    await userEvent.click(screen.getByRole('button', { name: 'Restart' }))
    await waitFor(() => expect(posts(spy)).toHaveLength(1))
    expect(posts(spy)[0]?.[0]).toBe('/api/v1/apps/web/restart')
  })

  it('stop always confirms and never offers the opt-out', async () => {
    window.localStorage.setItem('app-power:skip-restart-confirm', '1')
    const spy = setup()
    await userEvent.click(screen.getByText('toggle'))
    expect(posts(spy)).toHaveLength(0)
    expect(screen.queryByRole('checkbox')).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Stop' }))
    await waitFor(() => expect(posts(spy)).toHaveLength(1))
    expect(posts(spy)[0]?.[0]).toBe('/api/v1/apps/web/stop')
  })

  it('restart opt-out is remembered in this browser', async () => {
    const spy = setup()
    await userEvent.click(screen.getByText('restart'))
    await userEvent.click(screen.getByRole('checkbox'))
    await userEvent.click(screen.getByRole('button', { name: 'Restart' }))
    await waitFor(() => expect(posts(spy)).toHaveLength(1))
    expect(window.localStorage.getItem('app-power:skip-restart-confirm')).toBe(
      '1',
    )
  })
})

describe('useAppPower progress', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    resetPowerTransitions()
    toastAdd.mockClear()
  })

  function run(conditions: unknown[]) {
    const spy = vi.spyOn(globalThis, 'fetch').mockImplementation((input) =>
      Promise.resolve({
        ok: true,
        status: 200,
        json: () =>
          Promise.resolve(
            urlOf(input).endsWith('/deploys') ? conditions : { name: 'web' },
          ),
      } as unknown as Response),
    )
    window.localStorage.setItem('app-power:skip-restart-confirm', '1')
    const client = new QueryClient({
      defaultOptions: { mutations: { retry: false } },
    })
    render(
      <CommonI18n>
        <QueryClientProvider client={client}>
          <Harness app={{ name: 'web', suspended: false } as AppDetail} />
        </QueryClientProvider>
      </CommonI18n>,
    )
    return spy
  }

  const healthy = [{ Type: 'Ready', Status: 'True', Reason: 'Ok', Message: '' }]

  it('shows progress, blocks a second click, then reports restarted', async () => {
    const spy = run(healthy)
    await userEvent.click(screen.getByText('restart'))
    const button = await screen.findByRole('button', { name: 'Restarting...' })
    expect(button).toBeDisabled()
    await userEvent.click(button)
    const posts = spy.mock.calls.filter(([, init]) => init?.method === 'POST')
    expect(posts).toHaveLength(1)
    await waitFor(
      () =>
        expect(toastAdd).toHaveBeenCalledWith(
          expect.objectContaining({ title: '"web" restarted.' }),
        ),
      { timeout: 6000 },
    )
    expect(screen.getByRole('button', { name: 'restart' })).toBeEnabled()
  })

  it('reports the failure reason with a logs action', async () => {
    run([
      {
        Type: 'Ready',
        Status: 'False',
        Reason: 'ReadinessFailed',
        Message: 'exit 1',
      },
    ])
    await userEvent.click(screen.getByText('restart'))
    expect(screen.queryByRole('dialog')).toBeNull()
    await waitFor(
      () =>
        expect(toastAdd).toHaveBeenCalledWith(
          expect.objectContaining({
            title: '"web" did not come back',
            description: 'ReadinessFailed: exit 1',
            type: 'error',
          }),
        ),
      { timeout: 6000 },
    )
  })
})

interface ToastCall {
  title: string
  type: string
  description?: string
  actionProps?: unknown
}

describe('useAppPower false failures and timeout', () => {
  const healthy = [{ Type: 'Ready', Status: 'True', Reason: 'Ok', Message: '' }]
  const bad = (Reason: string) => [
    { Type: 'Ready', Status: 'False', Reason, Message: 'boom' },
  ]
  let current: unknown[] = healthy
  let unmount: () => void = () => undefined

  function start() {
    vi.spyOn(globalThis, 'fetch').mockImplementation((input) =>
      Promise.resolve({
        ok: true,
        status: 200,
        json: () =>
          Promise.resolve(
            urlOf(input).endsWith('/deploys') ? current : { name: 'web' },
          ),
      } as unknown as Response),
    )
    window.localStorage.setItem('app-power:skip-restart-confirm', '1')
    const client = new QueryClient({
      defaultOptions: { mutations: { retry: false } },
    })
    const view = render(
      <CommonI18n>
        <QueryClientProvider client={client}>
          <Harness app={{ name: 'web', suspended: false } as AppDetail} />
        </QueryClientProvider>
      </CommonI18n>,
    )
    unmount = view.unmount
  }

  async function advance(ms: number) {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(ms)
    })
  }

  async function click() {
    fireEvent.click(screen.getByText('restart'))
    await advance(0)
  }

  const calls = () => toastAdd.mock.calls.map(([arg]) => arg as ToastCall)
  const titles = () => calls().map((c) => c.title)

  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: false })
    toastAdd.mockClear()
    current = healthy
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
    window.localStorage.clear()
    resetPowerTransitions()
    POWER_TIMING.timeoutMs = 60000
  })

  it('does not report a terminal failure that clears within the streak', async () => {
    current = bad('ReadinessFailed')
    start()
    await click()
    await advance(120)
    current = healthy
    await advance(1000)
    expect(titles()).not.toContain('"web" did not come back')
    expect(titles()).toContain('"web" restarted.')
  })

  it('never reports a transient reason as a failure', async () => {
    current = bad('CrashLoopBackOff')
    start()
    await click()
    await advance(5000)
    expect(titles()).toHaveLength(0)
    expect(screen.getByRole('button', { name: 'Restarting...' })).toBeDisabled()
  })

  it('reports a persistent terminal failure once with the reason', async () => {
    current = bad('ExitedDuringStableWindow')
    start()
    await click()
    await advance(5000)
    const fails = calls().filter((c) => c.type === 'error')
    expect(fails).toHaveLength(1)
    expect(fails[0]?.description).toBe('ExitedDuringStableWindow: boom')
    expect(fails[0]?.actionProps).toBeDefined()
  })

  it('past the timeout shows Still restarting, re-enables, no failure', async () => {
    POWER_TIMING.timeoutMs = 1000
    current = bad('ReadinessPending')
    start()
    await click()
    await advance(500)
    expect(screen.getByRole('button', { name: 'Restarting...' })).toBeDisabled()
    await advance(3000)
    expect(titles()).toContain('"web" is still restarting')
    expect(
      screen.getByRole('button', { name: 'Still restarting...' }),
    ).toBeEnabled()
    expect(calls().some((c) => c.type === 'error')).toBe(false)
  })

  it('does not update state after unmount', async () => {
    const err = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    current = bad('ReadinessPending')
    start()
    await click()
    unmount()
    await advance(5000)
    expect(err).not.toHaveBeenCalled()
  })
})
