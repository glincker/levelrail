import { describe, expect, it } from 'vitest'
import { computeKeepChecklist, type KeepChecklistInput } from './keepChecklist'

function states(input: KeepChecklistInput) {
  return Object.fromEntries(
    computeKeepChecklist(input).map((i) => [i.id, i.state]),
  )
}

describe('computeKeepChecklist', () => {
  it.each<[string, KeepChecklistInput, Record<string, string>]>([
    [
      'fresh trial: nothing set up',
      { volumeSchedules: [false], alertCount: 0 },
      { domain: 'todo', backup: 'todo', alert: 'todo', placement: 'todo' },
    ],
    [
      'everything set up',
      {
        domains: ['ntfy.example.com'],
        volumeSchedules: [true, false],
        alertCount: 2,
        projectName: 'Ops',
        environmentName: 'production',
      },
      { domain: 'done', backup: 'done', alert: 'done', placement: 'done' },
    ],
    [
      'still loading',
      { volumeSchedules: [undefined] },
      {
        domain: 'todo',
        backup: 'loading',
        alert: 'loading',
        placement: 'todo',
      },
    ],
    [
      'no volumes to back up',
      { volumeSchedules: [], alertCount: 1 },
      { domain: 'todo', backup: 'none', alert: 'done', placement: 'todo' },
    ],
  ])('%s', (_name, input, want) => {
    expect(states(input)).toEqual(want)
  })

  it('reports scheduled out of total volumes', () => {
    const backup = computeKeepChecklist({
      volumeSchedules: [true, false, true],
    }).find((i) => i.id === 'backup')
    expect(backup?.values).toEqual({ count: 2, total: 3 })
  })

  it('marks a project with no environment', () => {
    const placement = computeKeepChecklist({
      volumeSchedules: [],
      projectName: 'Ops',
    }).find((i) => i.id === 'placement')
    expect(placement?.variant).toBe('projectOnly')
  })
})
