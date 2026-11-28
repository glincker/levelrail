import { renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AppDetail } from '../../types/appDetail'
import type { UrlReachability } from '../../lib/urlReachability'
import { useOverviewActions } from './useOverviewActions'

const state = vi.hoisted((): { reach: UrlReachability } => ({
  reach: { state: 'reachable', reason: 'ok' },
}))
const power = vi.hoisted(() => ({
  toggleRunning: vi.fn(),
  restart: vi.fn(),
}))
const toastAdd = vi.hoisted(() => vi.fn())

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))
vi.mock('@/components/ui/toast', () => ({
  toast: { add: toastAdd },
  toastAction: (children: string, onClick: () => void) => ({
    children,
    onClick,
  }),
}))
vi.mock('../../hooks/useRedeployApp', () => ({
  useRedeployApp: () => ({ redeploy: vi.fn(), isPending: false }),
}))
vi.mock('../../hooks/useCopyToClipboard', () => ({
  useCopyToClipboard: () => ({ copied: false, copy: vi.fn() }),
}))
vi.mock('../apps/useAppPower', () => ({ useAppPower: () => power }))
vi.mock('./useUrlReachability', () => ({
  useUrlReachability: () => ({ reach: state.reach, shareUrl: null }),
}))

const app = { name: 'web', suspended: false, image: 'x' } as AppDetail
const URL = 'https://a.example.com'

function openApp() {
  return renderHook(() => useOverviewActions(app, URL)).result.current.openApp
}

function lastToast() {
  const arg: unknown = toastAdd.mock.calls[0]?.[0]
  if (!arg) throw new Error('no toast shown')
  return arg as {
    title: string
    actionProps?: { children: string; onClick: () => void }
  }
}

describe('useOverviewActions openApp', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    toastAdd.mockClear()
    power.toggleRunning.mockClear()
    power.restart.mockClear()
    state.reach = { state: 'reachable', reason: 'ok' }
  })

  it('opens a reachable address', () => {
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    openApp()()
    expect(open).toHaveBeenCalledWith(URL, '_blank', 'noopener,noreferrer')
    expect(toastAdd).not.toHaveBeenCalled()
  })

  it('explains a stopped app with a Start action instead of navigating', () => {
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    state.reach = { state: 'down', reason: 'stopped' }
    openApp()()
    expect(open).not.toHaveBeenCalled()
    const arg = lastToast()
    expect(arg.title).toBe('status.openBlocked.stopped')
    expect(arg.actionProps?.children).toBe('status.openBlocked.start')
    arg.actionProps?.onClick()
    expect(power.toggleRunning).toHaveBeenCalled()
  })

  it('offers Restart when the app is not running', () => {
    state.reach = { state: 'down', reason: 'not_running' }
    openApp()()
    lastToast().actionProps?.onClick()
    expect(power.restart).toHaveBeenCalled()
  })

  it('states the local-only reason and only opens on Open anyway', () => {
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    state.reach = { state: 'local_only', reason: 'local_host' }
    openApp()()
    expect(open).not.toHaveBeenCalled()
    const arg = lastToast()
    expect(arg.title).toBe('status.openBlocked.local_only')
    arg.actionProps?.onClick()
    expect(open).toHaveBeenCalledWith(URL, '_blank', 'noopener,noreferrer')
  })
})
