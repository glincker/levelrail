import { act, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SitePreview } from './SitePreview'

const app = vi.hoisted(() => ({ domains: ['docs.localhost:5290'] as string[] }))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))
vi.mock('@tanstack/react-query', () => ({
  useQuery: () => ({ data: { domains: app.domains } }),
}))
vi.mock('../../queries/apps', () => ({ appDetailQueryOptions: () => ({}) }))

afterEach(() => vi.useRealTimers())

describe('SitePreview', () => {
  it('renders a sandboxed, non-interactive frame of the live URL', () => {
    const { container } = render(<SitePreview name="docs" />)
    const frame = container.querySelector('iframe')
    expect(frame?.getAttribute('src')).toBe('https://docs.localhost:5290')
    expect(frame?.getAttribute('sandbox')).toBe('allow-scripts')
    expect(frame?.className).toContain('pointer-events-none')
  })

  it('falls back to a placeholder when the frame never loads', () => {
    vi.useFakeTimers()
    const { container } = render(<SitePreview name="docs" />)
    act(() => {
      vi.advanceTimersByTime(10000)
    })
    expect(container.querySelector('iframe')).toBeNull()
    expect(screen.getByText('preview.unavailable')).toBeTruthy()
  })
})
