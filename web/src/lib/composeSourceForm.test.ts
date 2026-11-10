import { describe, expect, it } from 'vitest'
import { ApiError } from './apiError'
import {
  composePathProblem,
  composeSourceErrorKey,
  composeSourcePayload,
  emptyComposeSourceForm,
} from './composeSourceForm'

describe('composePathProblem', () => {
  it.each([
    ['', null],
    ['compose.yaml', null],
    ['deploy/stack.yml', null],
    ['./compose.yaml', null],
    ['/etc/compose.yaml', 'absolute'],
    ['C:/compose.yaml', 'absolute'],
    ['a\\b.yaml', 'absolute'],
    ['../compose.yaml', 'parent'],
    ['a/../../b.yml', 'parent'],
    ['compose.json', 'extension'],
    ['deploy/', 'extension'],
    ['.', 'notFile'],
    ['a\u0007.yaml', 'control'],
    ['a\u200b.yaml', 'control'],
    [`${'a'.repeat(510)}.yml`, 'tooLong'],
  ])('%s gives %s', (input, want) => {
    expect(composePathProblem(input)).toBe(want)
  })
})

describe('composeSourcePayload', () => {
  it('trims fields and omits an empty token', () => {
    const form = {
      ...emptyComposeSourceForm(),
      repoUrl: ' https://github.com/acme/stack.git ',
      branch: ' ',
      composePath: ' deploy/compose.yml ',
      triggerMode: 'release' as const,
    }
    expect(composeSourcePayload(form)).toEqual({
      repo_url: 'https://github.com/acme/stack.git',
      branch: undefined,
      compose_path: 'deploy/compose.yml',
      trigger_mode: 'release',
    })
  })

  it('sends a typed token', () => {
    const form = { ...emptyComposeSourceForm(), repoUrl: 'r', token: 't1' }
    expect(composeSourcePayload(form).token).toBe('t1')
  })
})

describe('composeSourceErrorKey', () => {
  it('maps the server refusals', () => {
    expect(
      composeSourceErrorKey(new ApiError(409, 'already has a git source')),
    ).toBe('kindConflict')
    expect(
      composeSourceErrorKey(
        new ApiError(409, 'this app is in a protected environment'),
      ),
    ).toBe('protected')
    expect(composeSourceErrorKey(new ApiError(423, 'frozen'))).toBe('frozen')
    expect(
      composeSourceErrorKey(
        new ApiError(
          400,
          'trigger_mode for a compose source must be push or release',
        ),
      ),
    ).toBe('pollNotAllowed')
    expect(
      composeSourceErrorKey(new ApiError(400, 'repo_url is required')),
    ).toBeNull()
    expect(composeSourceErrorKey(new Error('x'))).toBeNull()
  })
})
