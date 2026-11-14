import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ErrorBoundary } from './ErrorBoundary'

function Bomb({ boom }: { boom: boolean }) {
  if (boom) throw new Error('kaput')
  return <p>fine</p>
}

describe('ErrorBoundary', () => {
  it('renders the fallback on a crash and recovers when reset keys change', () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const ui = (boom: boolean, key: number) => (
      <ErrorBoundary
        resetKeys={[key]}
        fallback={(e) => <p>caught {e.message}</p>}
      >
        <Bomb boom={boom} />
      </ErrorBoundary>
    )
    const { rerender } = render(ui(true, 1))
    expect(screen.getByText('caught kaput')).toBeInTheDocument()
    rerender(ui(false, 1))
    expect(screen.getByText('caught kaput')).toBeInTheDocument()
    rerender(ui(false, 2))
    expect(screen.getByText('fine')).toBeInTheDocument()
  })
})
