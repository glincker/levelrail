import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ShortcutsDialog } from './ShortcutsDialog'
import { SHORTCUT_DOCS } from '@/lib/shortcuts'

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
})
