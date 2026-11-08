import { afterEach, beforeAll, describe, expect, it } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { SidebarProvider, useSidebar } from './sidebar'

function Probe() {
  const { state, toggleSidebar } = useSidebar()
  return (
    <button type="button" onClick={toggleSidebar}>
      {state}
    </button>
  )
}

describe('sidebar collapse persistence', () => {
  beforeAll(() => {
    window.matchMedia = ((q: string) => ({
      matches: false,
      media: q,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
    })) as unknown as typeof window.matchMedia
  })
  afterEach(() => localStorage.clear())

  it('stores the collapsed state and restores it on mount', () => {
    const first = render(
      <SidebarProvider>
        <Probe />
      </SidebarProvider>,
    )
    fireEvent.click(screen.getByRole('button'))
    expect(localStorage.getItem('sidebar_state')).toBe('false')
    first.unmount()
    render(
      <SidebarProvider>
        <Probe />
      </SidebarProvider>,
    )
    expect(screen.getByRole('button').textContent).toBe('collapsed')
  })
})
