import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { QueryClientProvider } from '@tanstack/react-query'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { ApiError } from '../../lib/apiError'
import type { BuildQueue, KillAllResult } from '../../queries/buildQueue'
import deploysEn from '../../locales/en/deploys.json'
import commonEn from '../../locales/en/common.json'
import { createTestQueryClient } from '@/test/testQueryClient'
import { BuildQueueBanner } from './BuildQueueBanner'
import { BuildQueueControls } from './BuildQueueControls'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['deploys', 'common'],
  defaultNS: 'deploys',
  resources: { en: { deploys: deploysEn, common: commonEn } },
  interpolation: { escapeValue: false },
})

let queue: BuildQueue | undefined
let canWrite = true
const queueMutate = vi.fn()
const killMutate = vi.fn()

vi.mock('../../queries/buildQueue', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('../../queries/buildQueue')>()
  return {
    ...actual,
    useBuildQueue: () => ({ data: queue }),
    useQueueAction: () => ({ mutate: queueMutate, isPending: false }),
    useKillAllBuilds: () => ({ mutate: killMutate, isPending: false }),
  }
})
vi.mock('@/hooks/useAbilities', () => ({
  useAbilities: () => ({ role: 'admin', can: () => canWrite }),
}))
vi.mock('@/hooks/useMediaQuery', () => ({ useMediaQuery: () => true }))
vi.mock('../env/StepUpDialog', () => ({
  StepUpDialog: ({
    scope,
    onVerified,
  }: {
    scope: unknown
    onVerified: () => void
  }) =>
    scope ? (
      <button type="button" onClick={onVerified}>
        verify step-up
      </button>
    ) : null,
}))
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({
      children,
      to,
      ...rest
    }: {
      children?: ReactNode
      to?: string
    } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
      <a href={to} {...rest}>
        {children}
      </a>
    ),
  }
})

function makeQueue(over: Partial<BuildQueue> = {}): BuildQueue {
  return {
    state: 'running',
    drained: false,
    slots: { used: 1, total: 2 },
    running: [
      { deploy_id: 'd1', app: 'web', phase: 'building', elapsed_seconds: 75 },
    ],
    queued: [{ deploy_id: 'd2', app: 'api', position: 1, wait_seconds: 30 }],
    recent: [],
    ...over,
  }
}

function wrap(node: ReactNode) {
  return (
    <QueryClientProvider client={createTestQueryClient()}>
      <I18nextProvider i18n={testI18n}>{node}</I18nextProvider>
    </QueryClientProvider>
  )
}

afterEach(() => {
  queue = undefined
  canWrite = true
  queueMutate.mockReset()
  killMutate.mockReset()
})

describe('queue state chip', () => {
  it.each([
    ['running', /Running, 1 of 2 slots/],
    ['paused', /Paused, 1 of 2 slots/],
    ['draining', /Draining, 1 of 2 slots/],
  ] as const)('shows %s with slots', (state, label) => {
    queue = makeQueue({ state })
    render(wrap(<BuildQueueControls />))
    expect(screen.getByText(label)).toBeInTheDocument()
  })

  it('offers Resume instead of Pause when paused', () => {
    queue = makeQueue({ state: 'paused' })
    render(wrap(<BuildQueueControls />))
    expect(
      screen.getByRole('button', { name: 'Resume builds' }),
    ).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Pause builds' })).toBeNull()
  })
})

describe('pause popover', () => {
  it('sends the optional reason', async () => {
    queue = makeQueue()
    render(wrap(<BuildQueueControls />))
    await userEvent.click(screen.getByRole('button', { name: 'Pause builds' }))
    await userEvent.type(
      await screen.findByLabelText('Reason (optional)'),
      'registry maintenance',
    )
    await userEvent.click(screen.getByRole('button', { name: 'Pause' }))
    expect(queueMutate).toHaveBeenCalledWith(
      { action: 'pause', reason: 'registry maintenance' },
      expect.anything(),
    )
  })
})

async function openKillAll() {
  queue = makeQueue()
  render(wrap(<BuildQueueControls />))
  await userEvent.click(screen.getByRole('button', { name: 'Kill all builds' }))
  return screen.findByRole('dialog')
}

// The trigger and the dialog confirm share a label; the confirm renders last.
function confirmButton(): HTMLElement {
  const all = screen.getAllByRole('button', { name: 'Kill all builds' })
  const last = all.at(-1)
  if (!last) throw new Error('no kill all button')
  return last
}

describe('kill all builds', () => {
  it('lists counts and apps and needs the typed word', async () => {
    const dialog = await openKillAll()
    expect(dialog).toHaveTextContent('1 running and 1 queued')
    expect(dialog).toHaveTextContent('web, api')
    const submit = confirmButton()
    expect(submit).toBeDisabled()
    await userEvent.type(screen.getByLabelText(/Type KILL/), 'KILL')
    expect(submit).toBeEnabled()
  })

  it('retries after a step-up and can also pause', async () => {
    await openKillAll()
    await userEvent.click(screen.getByRole('checkbox'))
    await userEvent.type(screen.getByLabelText(/Type KILL/), 'KILL')
    killMutate.mockImplementationOnce(
      (_input: unknown, opts: { onError: (e: Error) => void }) => {
        opts.onError(new ApiError(403, 'step up', { code: 'step_up_required' }))
      },
    )
    await userEvent.click(confirmButton())
    expect(killMutate).toHaveBeenCalledWith({ pause: true }, expect.anything())
    killMutate.mockImplementationOnce(
      (_input: unknown, opts: { onSuccess: (r: KillAllResult) => void }) => {
        opts.onSuccess({ canceled: [], failed: [], paused: true })
      },
    )
    fireEvent.click(await screen.findByText('verify step-up'))
    await waitFor(() => {
      expect(killMutate).toHaveBeenCalledTimes(2)
    })
  })

  it('explains a 409 and shows other errors', async () => {
    await openKillAll()
    await userEvent.type(screen.getByLabelText(/Type KILL/), 'KILL')
    const submit = confirmButton()
    killMutate.mockImplementationOnce(
      (_input: unknown, opts: { onError: (e: Error) => void }) => {
        opts.onError(new ApiError(409, 'conflict'))
      },
    )
    await userEvent.click(submit)
    expect(
      await screen.findByText(/queue changed while you confirmed/),
    ).toBeVisible()
    killMutate.mockImplementationOnce(
      (_input: unknown, opts: { onError: (e: Error) => void }) => {
        opts.onError(new ApiError(500, 'boom'))
      },
    )
    await userEvent.click(submit)
    expect(await screen.findByText('boom')).toBeVisible()
  })
})

describe('queue banner', () => {
  it('renders nothing while running', () => {
    queue = makeQueue()
    const { container } = render(wrap(<BuildQueueBanner />))
    expect(container).toBeEmptyDOMElement()
  })

  it('shows reason and a Resume action when paused', async () => {
    queue = makeQueue({
      state: 'paused',
      actor: 'alice',
      reason: 'registry down',
      since: new Date().toISOString(),
    })
    render(wrap(<BuildQueueBanner />))
    expect(screen.getByText('Builds are paused')).toBeInTheDocument()
    expect(screen.getByText(/registry down/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Resume builds' }))
    expect(queueMutate).toHaveBeenCalledWith(
      { action: 'resume', reason: undefined },
      expect.anything(),
    )
  })

  it('hides Resume without the ability', () => {
    queue = makeQueue({ state: 'draining' })
    canWrite = false
    render(wrap(<BuildQueueBanner />))
    expect(screen.getByText('Builds are draining')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Resume builds' })).toBeNull()
  })
})
