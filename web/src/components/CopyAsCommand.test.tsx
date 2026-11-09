import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { CopyAsCommand } from './CopyAsCommand'
import { BrandContext } from '../lib/brandContext'
import type { Brand } from '../types/brand'

const { copy, add } = vi.hoisted(() => ({ copy: vi.fn(), add: vi.fn() }))
vi.mock('../hooks/useCopyToClipboard', () => ({
  useCopyToClipboard: () => ({ copied: false, copy }),
}))
vi.mock('@/components/ui/toast', () => ({ toast: { add } }))

async function menuItem(i: number): Promise<HTMLElement> {
  const el = (await screen.findAllByRole('menuitem'))[i]
  if (!el) throw new Error(`menu item ${String(i)} missing`)
  return el
}

const brand = { BinaryName: 'acme', Name: 'Acme' } as Brand

describe('CopyAsCommand', () => {
  it('renders nothing outside the brand provider', () => {
    const { container } = render(
      <CopyAsCommand
        subject="web"
        actions={[{ kind: 'app.restart', app: 'web' }]}
      />,
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('copies the CLI command and the API call with a toast', async () => {
    render(
      <BrandContext.Provider value={brand}>
        <CopyAsCommand
          subject="my app"
          actions={[{ kind: 'app.restart', app: 'my app' }]}
        />
      </BrandContext.Provider>,
    )
    await userEvent.click(
      screen.getByRole('button', { name: 'copyCommand.trigger' }),
    )
    await userEvent.click(await menuItem(0))
    expect(copy).toHaveBeenCalledWith("acme-cli apps restart 'my app'")
    expect(add).toHaveBeenCalledWith(
      expect.objectContaining({ title: 'copyCommand.copiedCli' }),
    )

    await userEvent.click(
      screen.getByRole('button', { name: 'copyCommand.trigger' }),
    )
    await userEvent.click(await menuItem(1))
    expect(copy).toHaveBeenLastCalledWith(
      expect.stringContaining('Bearer $ACME_TOKEN'),
    )
  })
})
