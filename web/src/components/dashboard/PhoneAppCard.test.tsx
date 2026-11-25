import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import { PhoneAppCard } from './PhoneAppCard'
import type { AppListEntry } from '../../types/appDetail'
import { createTestQueryClient } from '@/test/testQueryClient'

const abilities = vi.hoisted(() => ({ granted: ['deploy'] as string[] }))
const restart = vi.hoisted(() => ({ mutate: vi.fn() }))

vi.mock('../../hooks/useAbilities', () => ({
  useAbilities: () => ({
    role: 'custom',
    can: (a: string) => abilities.granted.includes(a),
  }),
}))
vi.mock('../../queries/apps', () => ({
  useRestartApp: () => ({ mutate: restart.mutate, isPending: false }),
}))
vi.mock('@tanstack/react-router', () => ({
  Link: ({
    children,
    ...rest
  }: { children?: ReactNode } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
    <a {...rest}>{children}</a>
  ),
}))

const app = {
  name: 'web',
  image: 'nginx:1',
  status: { label: 'Running', variant: 'success' },
  summary: { ready: 'true', last_deploy_at: '2026-11-01T00:00:00Z' },
} as unknown as AppListEntry

function renderCard() {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <ul>
        <PhoneAppCard app={app} />
      </ul>
    </QueryClientProvider>,
  )
}

describe('PhoneAppCard', () => {
  it('shows state and health, and asks before restarting', async () => {
    abilities.granted = ['deploy']
    renderCard()
    expect(screen.getByText('Running')).toBeTruthy()
    expect(screen.getByText(/phoneHome.health.ready/)).toBeTruthy()
    await userEvent.click(
      screen.getByRole('button', { name: 'phoneHome.restart' }),
    )
    expect(restart.mutate).not.toHaveBeenCalled()
    await userEvent.click(
      within(screen.getByRole('dialog')).getByRole('button', {
        name: 'phoneHome.restart',
      }),
    )
    expect(restart.mutate).toHaveBeenCalledWith('web', expect.anything())
  })

  it('hides every action for a read-only token', () => {
    abilities.granted = ['read']
    renderCard()
    expect(screen.queryByRole('button')).toBeNull()
  })
})
