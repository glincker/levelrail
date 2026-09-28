import { describe, expect, it } from 'vitest'
import { drainFailures, drainWarnings } from './drainResult'
import type { DrainNodeResponse } from '../types/nodeDetail'

const base: DrainNodeResponse = {
  target_node_id: '',
  moved_services: [],
  moved_databases: [],
}

describe('drainFailures', () => {
  it('drops errors that are already listed as blocked', () => {
    expect(
      drainFailures({
        ...base,
        blocked: [
          { kind: 'app', name: 'llm', reason: 'no GPU node available' },
          { kind: 'model', name: 'chat', reason: 'cannot be moved' },
        ],
        errors: [
          'service llm: no GPU node available',
          'model chat: cannot be moved',
          'database main: disk full',
        ],
      }),
    ).toEqual(['database main: disk full'])
  })

  it('returns every error when nothing is blocked', () => {
    expect(drainFailures({ ...base, errors: ['service web: boom'] })).toEqual([
      'service web: boom',
    ])
    expect(drainFailures(base)).toEqual([])
  })
})

describe('drainWarnings', () => {
  it('returns warnings, or an empty list when absent', () => {
    expect(
      drainWarnings({ ...base, warnings: ['models could not be checked'] }),
    ).toEqual(['models could not be checked'])
    expect(drainWarnings(base)).toEqual([])
  })
})
