import { afterEach, describe, expect, it, vi } from 'vitest'
import { dismissTour, isTourDismissed, TOUR_STEPS } from './demoTour'

afterEach(() => {
  window.localStorage.clear()
  vi.restoreAllMocks()
})

describe('demo tour', () => {
  it('has five steps ending with the call to action', () => {
    expect(TOUR_STEPS).toHaveLength(5)
    expect(TOUR_STEPS.at(-1)?.id).toBe('finish')
  })

  it('persists dismissal', () => {
    expect(isTourDismissed()).toBe(false)
    dismissTour()
    expect(isTourDismissed()).toBe(true)
  })

  it('survives blocked storage', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    expect(isTourDismissed()).toBe(false)
    expect(() => {
      dismissTour()
    }).not.toThrow()
  })
})
