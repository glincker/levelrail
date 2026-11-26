import { describe, expect, it } from 'vitest'
import {
  DOCUMENT_ENGINES,
  EXPLORER_UNAVAILABLE_ENGINES,
  hasExplorer,
} from './engines'

describe('explorer engine capability', () => {
  it('keeps DocumentDB out of the document explorer', () => {
    expect(DOCUMENT_ENGINES.has('documentdb')).toBe(false)
    expect(EXPLORER_UNAVAILABLE_ENGINES.has('documentdb')).toBe(true)
    expect(hasExplorer('documentdb')).toBe(false)
  })

  it.each(['postgres', 'mongodb', 'redis', 'dynamodb', 'qdrant'])(
    '%s has an explorer',
    (engine) => {
      expect(hasExplorer(engine)).toBe(true)
    },
  )
})
