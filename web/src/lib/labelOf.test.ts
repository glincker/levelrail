import { describe, expect, it } from 'vitest'
import { labelOf, linkOf } from './labelOf'

const labels = { aws: 'AWS S3' }

describe('labelOf', () => {
  it('returns the mapped label', () => {
    expect(labelOf(labels, 'aws')).toBe('AWS S3')
  })
  it('falls back to the raw key for an unknown value', () => {
    expect(labelOf(labels, 'wasabi')).toBe('wasabi')
    expect(labelOf(labels, 'constructor')).toBe('constructor')
  })
})

describe('linkOf', () => {
  it('returns the link or nothing', () => {
    expect(linkOf({ a: 'https://a.test' }, 'a')).toBe('https://a.test')
    expect(linkOf({ a: 'https://a.test' }, 'b')).toBeUndefined()
  })
})
