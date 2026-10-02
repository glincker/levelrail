import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ShortcutsDialog } from './ShortcutsDialog'
import { SHORTCUT_DOCS } from '@/lib/shortcuts'
import { registerPageActions } from '@/lib/pageActions'

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

  it('shows a "this page" group for the current page\'s hinted actions', () => {
    const unregister = registerPageActions([
      { key: 'copy', label: 'Copy URL', icon: null, run: vi.fn(), hint: ['C'] },
      { key: 'noop', label: 'No hint action', icon: null, run: vi.fn() },
    ])
    render(<ShortcutsDialog open onOpenChange={vi.fn()} />)
    expect(screen.getByText('This page')).toBeInTheDocument()
    expect(screen.getByText('Copy URL')).toBeInTheDocument()
    expect(screen.queryByText('No hint action')).not.toBeInTheDocument()
    unregister()
  })

  it('leaves out the "this page" group when no page registered hinted actions', () => {
    render(<ShortcutsDialog open onOpenChange={vi.fn()} />)
    expect(screen.queryByText('This page')).not.toBeInTheDocument()
  })
})
