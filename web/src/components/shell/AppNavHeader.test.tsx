import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ApiError } from '../../lib/apiError'
import { AppNavHeader } from './AppNavHeader'

const state = vi.hoisted(() => ({ error: null as Error | null }))

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
}))
vi.mock('@tanstack/react-query', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-query')>()),
  useQuery: () => ({ data: undefined, error: state.error }),
}))
vi.mock('@/components/ui/sidebar', () => ({
  SidebarMenuButton: ({ children }: { children: React.ReactNode }) => (
    <button type="button">{children}</button>
  ),
  useSidebar: () => ({ state: 'expanded' }),
}))
vi.mock('../../queries/deploys', () => ({
  useDeployStatus: () => ({ data: [] }),
}))
vi.mock('./useRequestRateSpark', () => ({ useRequestRateSpark: () => [] }))
vi.mock('../../queries/appSleep', () => ({ useAppAsleep: () => false }))

describe('AppNavHeader', () => {
  it('renders the name from the route when the app is missing (404)', () => {
    state.error = new ApiError(404, 'app not found: ghost')
    render(<AppNavHeader name="ghost" />)
    expect(screen.getByText('ghost')).toBeTruthy()
  })

  it('still throws other errors', () => {
    state.error = new ApiError(500, 'boom')
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    expect(() => render(<AppNavHeader name="ghost" />)).toThrow('boom')
    spy.mockRestore()
  })
})
