import { describe, expect, it } from 'vitest'
import { parseVersion } from './versionLabel'

describe('parseVersion', () => {
  it('detects git-describe dev builds', () => {
    expect(parseVersion('v0.6.0-138-gabbfcdba5')).toEqual({
      kind: 'dev',
      base: 'v0.6.0',
      commits: 138,
      sha: 'abbfcdba5',
      dirty: false,
    })
  })
  it('keeps a pre-release base and the dirty flag', () => {
    expect(parseVersion('v0.7.0-beta.2-4-g1234567-dirty')).toMatchObject({
      kind: 'dev',
      base: 'v0.7.0-beta.2',
      commits: 4,
      dirty: true,
    })
  })
  it('passes plain releases and unknown strings through', () => {
    expect(parseVersion('v0.7.0')).toEqual({
      kind: 'release',
      version: 'v0.7.0',
    })
    expect(parseVersion('v0.8.0-rc.1')).toMatchObject({ kind: 'release' })
    expect(parseVersion('dev')).toEqual({ kind: 'raw', version: 'dev' })
    expect(parseVersion('')).toEqual({ kind: 'raw', version: '' })
  })
})
