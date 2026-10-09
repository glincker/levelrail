import { describe, expect, it } from 'vitest'
import { diffAppEnv } from './appEnvDiff'

describe('diffAppEnv', () => {
  it('splits keys and never exposes a secret value', () => {
    const d = diffAppEnv(
      {
        env: { A: '1', B: '2', ONLY_A: 'x', P: 'plain' },
        secretKeys: ['DB', 'S'],
      },
      { env: { A: '1', B: '3', ONLY_B: 'y' }, secretKeys: ['DB', 'P'] },
    )
    expect(d.same).toBe(2)
    expect(d.onlyA.map((e) => e.key)).toEqual(['ONLY_A', 'S'])
    expect(d.onlyA[1]).toMatchObject({ secret: true, a: undefined })
    expect(d.onlyB).toEqual([{ key: 'ONLY_B', secret: false, b: 'y' }])
    expect(d.differ).toContainEqual({ key: 'B', secret: false, a: '2', b: '3' })
    expect(d.differ).toContainEqual({ key: 'P', secret: true })
  })
})
