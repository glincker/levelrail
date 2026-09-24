import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { LogRow } from './LogRow'

function renderRow(line: string, expanded: boolean) {
  const onToggle = vi.fn()
  render(
    <LogRow
      logLine={{ id: 1, line, stream: 'stdout' }}
      index={0}
      start={0}
      expanded={expanded}
      onToggle={onToggle}
      measure={() => undefined}
    />,
  )
  return { onToggle }
}

describe('LogRow', () => {
  afterEach(() => {
    cleanup()
  })

  it('shows a level tag and toggles on click', async () => {
    const { onToggle } = renderRow('WARN disk low', false)
    expect(screen.getByText('WRN')).toBeInTheDocument()
    await userEvent.setup().click(screen.getByRole('button'))
    expect(onToggle).toHaveBeenCalledOnce()
  })

  it('pretty prints JSON when expanded', () => {
    renderRow('{"level":"error","msg":"boom"}', true)
    expect(
      screen.getByText((c) => c.includes('"msg": "boom"')),
    ).toBeInTheDocument()
    expect(screen.getByText('Copy line')).toBeInTheDocument()
  })

  it('shows non-JSON lines in full when expanded', () => {
    renderRow('plain text line', true)
    expect(screen.getAllByText('plain text line')).toHaveLength(2)
  })
})
