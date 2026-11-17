import { describe, expect, it } from 'vitest'
import { pageProblems, pageState } from './deploymentsPageState'
import type { BuildRunner } from '../../queries/buildRunners'

const runner = (status: BuildRunner['status']): BuildRunner => ({
  id: 'r',
  name: 'r',
  kind: 'node',
  status,
  health: 'ok',
  slots_used: 0,
  slots_total: 2,
  waiting: 0,
  load: 0,
  pressure: 'none',
  labels: null,
})

describe('pageState', () => {
  it('is empty with no history and no builds', () => {
    expect(pageState({ hasHistory: false, buildsActive: false })).toBe('empty')
  })
  it('is idle with history and no builds', () => {
    expect(pageState({ hasHistory: true, buildsActive: false })).toBe('idle')
  })
  it('is active whenever builds run or wait', () => {
    expect(pageState({ hasHistory: false, buildsActive: true })).toBe('active')
    expect(pageState({ hasHistory: true, buildsActive: true })).toBe('active')
  })
})

describe('pageProblems', () => {
  it('reports nothing when healthy', () => {
    expect(
      pageProblems({ streamState: 'live', runners: [runner('ready')] }),
    ).toEqual({ streamDown: false, runnerProblem: false })
  })
  it('flags a reconnecting stream and an unhealthy runner', () => {
    expect(
      pageProblems({
        streamState: 'reconnecting',
        runners: [runner('offline')],
      }),
    ).toEqual({ streamDown: true, runnerProblem: true })
  })
})
