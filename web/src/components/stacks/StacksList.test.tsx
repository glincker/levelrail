import { describe, expect, it, vi } from 'vitest'
import { fireEvent, screen } from '@testing-library/react'
import { StacksList } from './StacksList'
import { renderWithProviders, stack } from './stacksTestUtils'

describe('StacksList', () => {
  it('shows latest revision, status and lock, pin and drift indicators', () => {
    renderWithProviders(
      <StacksList
        stacks={[
          stack({
            name: 'shop',
            latest_revision: 4,
            latest_status: 'partial',
            pinned_revision: 2,
            drift_policy: 'lock',
            lock: {
              holder: 'ci',
              revision: 4,
              expires_at: '2026-10-25T00:00:00Z',
            },
          }),
        ]}
        onOpen={vi.fn()}
      />,
    )
    expect(screen.getAllByText('r4').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Partial').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Locked').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Pinned to r2').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Drift locks').length).toBeGreaterThan(0)
  })

  it('opens a stack by name', () => {
    const onOpen = vi.fn()
    renderWithProviders(
      <StacksList stacks={[stack({ name: 'shop' })]} onOpen={onOpen} />,
    )
    const [open] = screen.getAllByRole('button', { name: 'shop' })
    fireEvent.click(open as HTMLElement)
    expect(onOpen).toHaveBeenCalledWith('shop')
  })
})

describe('StacksList scope', () => {
  it('shows a Project or Install badge per stack', () => {
    renderWithProviders(
      <StacksList
        stacks={[
          stack({ name: 'shop' }),
          stack({ name: 'platform', scope: 'install', project: '' }),
        ]}
        onOpen={vi.fn()}
      />,
    )
    expect(screen.getAllByText('Project').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Install').length).toBeGreaterThan(0)
  })
})
