import { describe, expect, it, vi } from 'vitest'
import { runConnectionSteps } from './canvasApply'
import type { ConnectionStep } from './canvasDraft'

const steps: ConnectionStep[] = [
  { kind: 'connect', app: 'a', database: 'd', envVar: 'X' },
  { kind: 'disconnect', app: 'b', database: 'd', envVar: 'Y' },
  { kind: 'connect', app: 'c', database: 'd', envVar: '' },
]

describe('runConnectionSteps', () => {
  it('runs every step in order when all succeed', async () => {
    const connect = vi.fn().mockResolvedValue(undefined)
    const disconnect = vi.fn().mockResolvedValue(undefined)
    const out = await runConnectionSteps(steps, { connect, disconnect })
    expect(out.map((r) => r.status)).toEqual(['applied', 'applied', 'applied'])
    expect(connect).toHaveBeenCalledWith('a', 'd', 'X')
    expect(disconnect).toHaveBeenCalledWith('b', 'Y')
  })

  it('stops at the first failure and skips the rest', async () => {
    const connect = vi.fn().mockResolvedValue(undefined)
    const disconnect = vi.fn().mockRejectedValue(new Error('unknown database'))
    const out = await runConnectionSteps(steps, { connect, disconnect })
    expect(out.map((r) => r.status)).toEqual(['applied', 'failed', 'skipped'])
    expect(out[1]?.error).toBe('unknown database')
    expect(connect).toHaveBeenCalledTimes(1)
  })
})
