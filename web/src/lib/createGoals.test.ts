import { describe, expect, it } from 'vitest'
import { GOALS, goalById, goalsForScope } from './createGoals'

describe('createGoals', () => {
  it('gives every goal at least one category and a unique id', () => {
    expect(new Set(GOALS.map((g) => g.id)).size).toBe(GOALS.length)
    for (const g of GOALS) expect(g.categories.length).toBeGreaterThan(0)
  })

  it('prefills a port for every app goal', () => {
    for (const g of GOALS.filter((x) => x.kind === 'app')) {
      expect(g.defaults?.port).toBeGreaterThan(0)
    }
  })

  it('narrows goals by wizard scope', () => {
    expect(goalsForScope('databases').map((g) => g.id)).toEqual(['database'])
    expect(goalsForScope('applications').some((g) => g.id === 'database')).toBe(
      false,
    )
    expect(goalsForScope(undefined)).toHaveLength(GOALS.length)
    expect(goalById('api')?.defaults?.healthPath).toBe('/healthz')
  })
})
