import { act, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AllI18n } from '../../test/allI18n'
import { FormHero } from './FormHero'

function mockReducedMotion(reduce: boolean) {
  vi.stubGlobal(
    'matchMedia',
    vi.fn().mockImplementation(() => ({
      matches: reduce,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  )
}

function renderHero() {
  return render(
    <AllI18n>
      <FormHero compact={false} />
    </AllI18n>,
  )
}

describe('FormHero playback', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('animates by default and shows the caption', () => {
    mockReducedMotion(false)
    renderHero()
    expect(screen.getByTestId('form-hero').dataset.motion).toBe('on')
    expect(screen.getByRole('img')).toBeTruthy()
  })

  it('shows a static final frame under reduced motion', () => {
    mockReducedMotion(true)
    renderHero()
    expect(screen.getByTestId('form-hero').dataset.motion).toBe('off')
  })

  it('pauses while the tab is hidden and resumes when visible', () => {
    mockReducedMotion(false)
    renderHero()
    const hero = screen.getByTestId('form-hero')
    expect(hero.dataset.paused).toBe('false')
    const visibility = vi.spyOn(document, 'visibilityState', 'get')
    act(() => {
      visibility.mockReturnValue('hidden')
      document.dispatchEvent(new Event('visibilitychange'))
    })
    expect(hero.dataset.paused).toBe('true')
    act(() => {
      visibility.mockReturnValue('visible')
      document.dispatchEvent(new Event('visibilitychange'))
    })
    expect(hero.dataset.paused).toBe('false')
  })
})
