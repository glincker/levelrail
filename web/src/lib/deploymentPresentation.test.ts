import { describe, expect, it } from 'vitest'
import {
  canRedeploy,
  commitSubject,
  statusView,
} from './deploymentPresentation'
import type { Deployment } from '../types/deployment'

describe('commitSubject', () => {
  it('keeps only the first line of a squash-merge body', () => {
    expect(
      commitSubject('release: TLS fix (#488)\n\n* feat: more\nlong body'),
    ).toBe('release: TLS fix (#488)')
  })

  it('caps a very long first line', () => {
    const out = commitSubject('x'.repeat(300))
    expect(out.length).toBe(120)
    expect(out.endsWith('…')).toBe(true)
  })

  it('returns short single-line messages unchanged', () => {
    expect(commitSubject('fix: typo')).toBe('fix: typo')
  })
})

describe('refused deploys', () => {
  it('has its own warning status and can be retried', () => {
    expect(statusView('refused').tone).toBe('warning')
    const d = { status: 'refused', image: 'nginx:1' } as Deployment
    expect(canRedeploy(d)).toBe(true)
  })
})
