import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { recordModeUse } from '@/lib/paletteModeUse'
import { ModePills } from './ModePills'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key.split('.').slice(-2, -1)[0] ?? key,
  }),
}))

beforeEach(() => window.localStorage.clear())

describe('ModePills', () => {
  it('shows every command as a pill and marks the active one', () => {
    render(<ModePills active="docs" onPick={vi.fn()} />)
    expect(screen.getAllByRole('button')).toHaveLength(4)
    expect(screen.getByRole('button', { pressed: true })).toBeInTheDocument()
  })

  it('puts the most used command first', () => {
    recordModeUse('docker')
    recordModeUse('docker')
    render(<ModePills active={null} onPick={vi.fn()} />)
    expect(screen.getAllByRole('button')[0]).toHaveTextContent('docker')
  })

  it('starts the command with its prefix when a pill is clicked', async () => {
    const onPick = vi.fn()
    render(<ModePills active={null} onPick={onPick} />)
    await userEvent.click(screen.getAllByRole('button')[0] as HTMLElement)
    expect(onPick).toHaveBeenCalledWith('docs', '/')
  })
})
