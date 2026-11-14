import { describe, expect, it } from 'vitest'
import { errorReport } from './errorReport'

describe('errorReport', () => {
  it('caps a huge message and the whole report', () => {
    const text = errorReport(new Error('failure '.repeat(10000)), 'Details')
    expect(text.length).toBeLessThanOrEqual(4100)
    expect(text).toContain('[truncated]')
    expect(text.startsWith('Details')).toBe(true)
  })
  it('leaves a short error intact', () => {
    const text = errorReport(new Error('boom'), 'Details', 'v1')
    expect(text).toContain('message: boom')
    expect(text).toContain('version: v1')
    expect(text).not.toContain('[truncated]')
  })
})
