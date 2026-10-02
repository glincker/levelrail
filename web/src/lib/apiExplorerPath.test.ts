import { describe, expect, it } from 'vitest'
import { pathParamNames } from './apiExplorerPath'

describe('pathParamNames', () => {
  it('extracts every {param} segment in order', () => {
    expect(pathParamNames('/api/v1/apps/{name}/secrets/{key}')).toEqual([
      'name',
      'key',
    ])
  })

  it('returns an empty array for a path with no parameters', () => {
    expect(pathParamNames('/api/v1/apps')).toEqual([])
  })
})
