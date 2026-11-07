import { describe, expect, it } from 'vitest'
import { hueClasses, hueForGroup, hueForItem } from './navHues'

describe('navHues', () => {
  it('tints top level items by what they are', () => {
    expect(hueForItem('dashboard', 'primary')).toBe('overview')
    expect(hueForItem('databases', 'primary')).toBe('data')
    expect(hueForItem('nodes', 'primary')).toBe('fleet')
  })

  it('falls back to the group hue and to none', () => {
    expect(hueForItem('dns', 'traffic')).toBe('network')
    expect(hueForItem('settings')).toBeUndefined()
    expect(hueClasses(hueForGroup('access'))?.dot).toBe(
      'bg-sidebar-hue-security',
    )
  })
})
