import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ShortcutsDialog } from './ShortcutsDialog'
import { SHORTCUT_DOCS } from '@/lib/shortcuts'

let experimentalOn: string[] = ['ai-models', 'load-balancer']
vi.mock('@/hooks/useExperimental', () => ({
  useExperimentalFeatures: () => experimentalOn,
}))

describe('ShortcutsDialog', () => {
  it('lists every shortcut in an accessible dialog', () => {
    render(<ShortcutsDialog open onOpenChange={vi.fn()} />)
    expect(
      screen.getByRole('dialog', { name: 'Keyboard shortcuts' }),
    ).toBeInTheDocument()
    for (const s of SHORTCUT_DOCS) {
      expect(screen.getByText(s.description)).toBeInTheDocument()
    }
    expect(screen.getByText('Open the command palette')).toBeInTheDocument()
    expect(screen.getByText('Go to Backups')).toBeInTheDocument()
  })

  it('renders nothing when closed', () => {
    render(<ShortcutsDialog open={false} onOpenChange={vi.fn()} />)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('leaves out chords for features that are off', () => {
    experimentalOn = []
    render(<ShortcutsDialog open onOpenChange={vi.fn()} />)
    expect(screen.queryByText('Go to AI models')).not.toBeInTheDocument()
    expect(screen.queryByText('Go to Load balancers')).not.toBeInTheDocument()
    expect(screen.getByText('Go to Backups')).toBeInTheDocument()
    experimentalOn = ['ai-models', 'load-balancer']
  })
})
