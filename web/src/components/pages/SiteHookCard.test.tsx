import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SiteHookCard from './SiteHookCard'

const h = vi.hoisted(() => ({ mint: vi.fn(), enabled: false }))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))
vi.mock('../../queries/pagesHook', () => ({
  useSiteHook: () => ({
    isSuccess: true,
    isPending: false,
    isError: false,
    data: { enabled: h.enabled },
  }),
  useMintSiteHook: () => ({ mutate: h.mint, isPending: false, error: null }),
  useRevokeSiteHook: () => ({ mutate: vi.fn(), isPending: false, error: null }),
}))

beforeEach(() => {
  h.mint.mockReset()
  vi.spyOn(window, 'confirm').mockReturnValue(true)
})

describe('SiteHookCard', () => {
  it('creates without the rotate intent when no hook exists', () => {
    h.enabled = false
    render(<SiteHookCard name="docs" />)
    fireEvent.click(screen.getByText('hook.create'))
    expect(h.mint.mock.calls[0]?.[0]).toBe(false)
  })

  it('sends the rotate intent only when a hook exists', () => {
    h.enabled = true
    render(<SiteHookCard name="docs" />)
    fireEvent.click(screen.getByText('hook.rotate'))
    expect(h.mint.mock.calls[0]?.[0]).toBe(true)
  })
})
