import { describe, expect, it } from 'vitest'
import {
  essentialSplit,
  guideStep,
  showGuideEntryPoints,
  stepGroups,
  type SetupGuide,
  type SetupGuideStep,
} from './setupGuide'

const step = (
  id: SetupGuideStep['id'],
  state: SetupGuideStep['state'],
): SetupGuideStep => ({
  id,
  title: id,
  state,
  optional: false,
  estimate_minutes: 1,
  href: '/',
})

const base: SetupGuide = {
  steps: [
    step('secure_admin', 'done'),
    step('first_app', 'todo'),
    step('tls', 'in_progress'),
    step('teammate', 'skipped'),
    step('dns', 'not_applicable'),
  ],
  ready_percent: 20,
  done: 1,
  total: 4,
  next_step: 'first_app',
  complete: false,
  hidden: false,
}

describe('stepGroups', () => {
  it('splits focus, open and settled steps', () => {
    const g = stepGroups(base)
    expect(g.next?.id).toBe('first_app')
    expect(g.todo.map((s) => s.id)).toEqual(['tls'])
    expect(g.settled.map((s) => s.id)).toEqual([
      'secure_admin',
      'teammate',
      'dns',
    ])
  })

  it('has no focus step when nothing is next', () => {
    expect(stepGroups({ ...base, next_step: undefined }).next).toBeUndefined()
  })
})

describe('essentialSplit', () => {
  it('puts the first app first and orders the production steps after it', () => {
    const g = essentialSplit({
      ...base,
      steps: [
        step('tls', 'todo'),
        step('first_app', 'todo'),
        step('secure_admin', 'done'),
        step('public_address', 'todo'),
        step('git_provider', 'todo'),
        step('teammate', 'todo'),
      ],
    })
    expect(g.essential.map((s) => s.id)).toEqual(['first_app'])
    expect(g.more.map((s) => s.id)).toEqual([
      'public_address',
      'secure_admin',
      'git_provider',
      'tls',
      'teammate',
    ])
  })
})

describe('showGuideEntryPoints', () => {
  it.each([
    ['loading', undefined, false],
    ['visible', base, true],
    ['hidden', { ...base, hidden: true }, false],
    ['complete', { ...base, complete: true }, false],
  ] as const)('%s', (_name, guide, want) => {
    expect(showGuideEntryPoints(guide)).toBe(want)
  })
})

describe('guideStep', () => {
  it('finds a step by id', () => {
    expect(guideStep(base, 'tls')?.state).toBe('in_progress')
    expect(guideStep(undefined, 'tls')).toBeUndefined()
  })
})
