import { describe, expect, it } from 'vitest'
import { firstFailure } from './pipelineFailure'
import { initialParamValues } from './pipelineParams'
import type { PipelineJob } from '../types/pipelines'

describe('initialParamValues', () => {
  it('seeds defaults and treats a boolean without one as false', () => {
    expect(
      initialParamValues([
        { name: 'env', type: 'choice', choices: ['a', 'b'], default: 'b' },
        { name: 'dry', type: 'boolean' },
        { name: 'note' },
      ]),
    ).toEqual({ env: 'b', dry: 'false', note: '' })
  })
})

describe('firstFailure', () => {
  const job = (key: string, status: PipelineJob['status']): PipelineJob => ({
    key,
    name: key,
    needs: [],
    status,
    attempt: 0,
    steps: [
      { index: 0, name: 'ok', kind: 'run', status: 'succeeded', attempt: 0 },
      {
        index: 1,
        name: 'make',
        kind: 'run',
        status: status === 'failed' ? 'failed' : 'pending',
        exit_code: 2,
        attempt: 0,
      },
    ],
  })
  it('returns the first failed job with its failed step and exit code', () => {
    const got = firstFailure([job('a', 'succeeded'), job('b', 'failed')])
    expect(got?.job.key).toBe('b')
    expect(got?.stepName).toBe('make')
    expect(got?.code).toBe(2)
  })
  it('returns null when nothing failed', () => {
    expect(firstFailure([job('a', 'succeeded')])).toBeNull()
  })
})
