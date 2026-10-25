import { describe, expect, it } from 'vitest'
import { envKind, imagePin, summarizeImagePins } from './templateTrust'

describe('imagePin', () => {
  it('detects digests, versions and floating tags', () => {
    expect(imagePin('ghcr.io/a/b:1.2@sha256:abc')).toBe('digest')
    expect(imagePin('postgres:16')).toBe('tag')
    expect(imagePin('nginx:latest')).toBe('floating')
    expect(imagePin('nginx')).toBe('floating')
    expect(imagePin('localhost:5000/app')).toBe('floating')
    expect(imagePin('localhost:5000/app:2')).toBe('tag')
  })
})

describe('summarizeImagePins', () => {
  it('counts unique images only', () => {
    expect(summarizeImagePins(['a:1', 'a:1', 'b', ''])).toEqual({
      digest: 0,
      tag: 1,
      floating: 1,
      total: 2,
    })
  })
})

describe('envKind', () => {
  it('prefers generated over required', () => {
    expect(envKind({ generated: true, required: true })).toBe('generated')
    expect(envKind({ required: true })).toBe('required')
    expect(envKind({})).toBe('optional')
  })
})
