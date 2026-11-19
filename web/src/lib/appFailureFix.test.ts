import { describe, expect, it } from 'vitest'
import { fixTarget } from './appFailureFix'

describe('fixTarget', () => {
  it('sends probe failures to the health tab', () => {
    expect(
      fixTarget({ reason: 'Ready', message: 'readiness probe failing' }, false),
    ).toBe('health')
  })
  it('sends certificate failures to domains', () => {
    expect(fixTarget({ message: 'TLS certificate expired' }, false)).toBe(
      'domains',
    )
  })
  it('defaults to the deploy logs', () => {
    expect(fixTarget({ message: 'image pull failed' }, false)).toBe('deploys')
  })
  it('sends static sites to Pages', () => {
    expect(fixTarget({ message: 'readiness probe failing' }, true)).toBe(
      'pages',
    )
  })
})
