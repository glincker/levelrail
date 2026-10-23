import { describe, expect, it } from 'vitest'
import type { GlobalEnvironment } from '../types/environment'
import { defaultEnvironmentFor, environmentsFor } from './scopeChoices'

const env = (
  id: string,
  project_id: string,
  kind: GlobalEnvironment['kind'],
  scope: GlobalEnvironment['scope'] = 'project',
) => ({ id, project_id, kind, scope, name: id }) as GlobalEnvironment

const ALL = [
  env('prod-a', 'a', 'production'),
  env('dev-a', 'a', 'dev'),
  env('prod-b', 'b', 'production'),
  env('shared', '', 'test', 'global'),
]

describe('scope choices', () => {
  it('offers a project its own and global environments only', () => {
    expect(environmentsFor(ALL, 'a').map((e) => e.id)).toEqual([
      'prod-a',
      'dev-a',
      'shared',
    ])
    expect(environmentsFor(ALL, '')).toHaveLength(4)
  })

  it('defaults to production when a project has several environments', () => {
    expect(defaultEnvironmentFor(ALL, 'a')).toBe('prod-a')
  })

  it('leaves all selected when a project has a single environment', () => {
    expect(defaultEnvironmentFor([env('prod-b', 'b', 'production')], 'b')).toBe(
      '',
    )
  })
})
