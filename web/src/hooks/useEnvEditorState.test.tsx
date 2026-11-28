import { createElement, type ReactNode } from 'react'
import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AppDetail } from '../types/appDetail'
import { EnvVersionConflictError } from '../queries/envApply'
import { useEnvEditorState } from './useEnvEditorState'

const applyEnv = vi.fn()
vi.mock('../queries/envApply', async (orig) => ({
  ...(await orig<typeof import('../queries/envApply')>()),
  applyEnv: (...args: unknown[]) => applyEnv(...args) as unknown,
}))
vi.mock('../queries/secrets', () => ({
  secretKeys: { list: (n: string) => ['secrets', n] },
  useSecretKeys: () => ({
    data: [{ key: 'TOKEN', locked: true }],
    isLoading: false,
  }),
}))

const app = {
  name: 'demo',
  env: { A: '1' },
  env_meta: { A: { sensitive: false, set: true, value_redacted: false } },
  secret_env: ['TOKEN'],
  env_version: 4,
} as unknown as AppDetail

function setup() {
  const client = new QueryClient()
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client }, children)
  return renderHook(() => useEnvEditorState(app), { wrapper })
}

function editA(hook: ReturnType<typeof setup>) {
  const row = hook.result.current.rows.find((r) => r.key === 'A')
  if (!row) throw new Error('row A missing')
  act(() => {
    hook.result.current.updateRow({ ...row, value: '2' })
  })
}

describe('useEnvEditorState', () => {
  beforeEach(() => {
    applyEnv.mockReset()
    sessionStorage.clear()
  })

  it('saves with one apply request carrying the version read at load', async () => {
    applyEnv.mockResolvedValue({ changed: true, env_version: 5, plan: [] })
    const hook = setup()
    editA(hook)
    await act(async () => {
      await hook.result.current.save()
    })
    expect(applyEnv).toHaveBeenCalledTimes(1)
    expect(applyEnv.mock.calls[0]?.[1]).toMatchObject({
      expected_version: 4,
      set: { A: '2' },
    })
  })

  it('a stale version opens the conflict and keeps the draft, also after reload', async () => {
    applyEnv.mockRejectedValue(new EnvVersionConflictError('stale', 9, ['A']))
    const hook = setup()
    editA(hook)
    await act(async () => {
      await hook.result.current.save().catch(() => undefined)
    })
    expect(hook.result.current.conflict).toEqual({
      currentVersion: 9,
      keys: ['A'],
    })
    expect(hook.result.current.dirty).toBe(true)
    const reloaded = setup()
    await waitFor(() => {
      expect(
        reloaded.result.current.rows.find((r) => r.key === 'A')?.value,
      ).toBe('2')
    })
  })

  it('a failed save leaves the draft intact', async () => {
    applyEnv.mockRejectedValue(new Error('boom'))
    const hook = setup()
    editA(hook)
    await act(async () => {
      await hook.result.current.save().catch(() => undefined)
    })
    expect(hook.result.current.dirty).toBe(true)
  })

  it('a locked secret needs its tick before saving', () => {
    const hook = setup()
    const row = hook.result.current.rows.find((r) => r.key === 'TOKEN')
    if (!row) throw new Error('row TOKEN missing')
    act(() => {
      hook.result.current.updateRow({ ...row, value: 'new-secret' })
    })
    expect(hook.result.current.lockedPending).toEqual(['TOKEN'])
    expect(hook.result.current.lockedBlocked).toBe(true)
    act(() => {
      hook.result.current.toggleReplaceLocked('TOKEN', true)
    })
    expect(hook.result.current.lockedBlocked).toBe(false)
  })
})
