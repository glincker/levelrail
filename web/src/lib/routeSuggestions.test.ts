import { describe, expect, it } from 'vitest'
import { suggestPages } from './routeSuggestions'

describe('suggestPages', () => {
  it('points a guessed top level path at the page one level deeper', () => {
    const paths = suggestPages('/registry').map((p) => p.path)
    expect(paths).toContain('/settings/registry')
  })

  it('returns nothing for the root path', () => {
    expect(suggestPages('/')).toEqual([])
  })
})
