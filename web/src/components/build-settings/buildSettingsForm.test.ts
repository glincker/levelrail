import { describe, expect, it } from 'vitest'
import {
  draftFrom,
  savePayload,
  validateDraft,
  type BuildSettings,
} from './buildSettingsForm'

const base: BuildSettings = {
  builder: 'dockerfile',
  dockerfile_path: 'Dockerfile',
  target: '',
  no_cache: false,
  provider: '',
  has_git_source: true,
  providers: ['golang', 'node'],
  max_value_bytes: 32,
}

describe('build settings form', () => {
  it('rejects values outside the allowlists', () => {
    const d = draftFrom(base)
    expect(validateDraft({ ...d, target: 'a b' }, base).target).toBe(
      'targetCharset',
    )
    expect(validateDraft({ ...d, target: '$(id)' }, base).target).toBe(
      'targetCharset',
    )
    expect(validateDraft({ ...d, target: 'x'.repeat(33) }, base).target).toBe(
      'tooLong',
    )
    expect(
      validateDraft({ ...d, dockerfile_path: '../Dockerfile' }, base)
        .dockerfile_path,
    ).toBe('pathRelative')
    expect(
      validateDraft({ ...d, dockerfile_path: '/etc/passwd' }, base)
        .dockerfile_path,
    ).toBe('pathRelative')
    expect(validateDraft({ ...d, provider: 'rust' }, base).provider).toBe(
      'provider',
    )
    expect(
      validateDraft({ ...d, target: 'runtime', provider: 'node' }, base),
    ).toEqual({})
  })

  it('sends only changed keys', () => {
    const d = { ...draftFrom(base), target: ' runtime ', no_cache: true }
    expect(savePayload(d, base)).toEqual({ target: 'runtime', no_cache: true })
    expect(savePayload(draftFrom(base), base)).toEqual({})
  })

  it('leaves git source fields out without a git source', () => {
    const s = { ...base, has_git_source: false }
    const d = {
      ...draftFrom(s),
      builder: 'railpack',
      dockerfile_path: 'x/Dockerfile',
    }
    expect(savePayload(d, s)).toEqual({})
  })
})
