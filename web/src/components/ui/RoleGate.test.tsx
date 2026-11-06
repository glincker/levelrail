import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { CommonI18n } from '@/test/commonI18n'
import { Button } from '@/components/ui/button'
import { TooltipProvider } from '@/components/ui/tooltip'
import type { Ability } from '@/types/token'
import { RoleGate } from './RoleGate'

const granted = vi.hoisted(() => ({ list: [] as string[] }))

vi.mock('@/hooks/useAbilities', () => ({
  useAbilities: () => ({
    role: null,
    can: (a: string) =>
      granted.list.includes(a) || granted.list.includes('root'),
  }),
}))

function setup(abilities: string[], ability: Ability) {
  granted.list = abilities
  const onClick = vi.fn()
  render(
    <CommonI18n>
      <TooltipProvider>
        <RoleGate ability={ability}>
          <Button onClick={onClick}>Prune</Button>
        </RoleGate>
      </TooltipProvider>
    </CommonI18n>,
  )
  return { onClick, button: screen.getByRole('button', { name: 'Prune' }) }
}

describe('RoleGate', () => {
  it('leaves the control alone for an admin', () => {
    const { button, onClick } = setup(['root'], 'root')
    expect(button).not.toHaveAttribute('aria-disabled')
    fireEvent.click(button)
    expect(onClick).toHaveBeenCalledOnce()
  })

  it('lets an operator use an operator control', () => {
    const { button, onClick } = setup(['read', 'write'], 'write')
    fireEvent.click(button)
    expect(onClick).toHaveBeenCalledOnce()
  })

  it('gates an operator out of an admin control, still focusable', () => {
    const { button, onClick } = setup(['read', 'write'], 'root')
    expect(button).toHaveAttribute('aria-disabled', 'true')
    button.focus()
    expect(button).toHaveFocus()
    fireEvent.click(button)
    expect(onClick).not.toHaveBeenCalled()
  })

  it('gates a viewer out of operator work and names the role on focus', async () => {
    const { button, onClick } = setup(['read'], 'write')
    fireEvent.click(button)
    expect(onClick).not.toHaveBeenCalled()
    fireEvent.focus(button)
    expect(await screen.findByText('Needs operator')).toBeInTheDocument()
  })

  it('says Needs admin for a root control', async () => {
    const { button } = setup(['read'], 'root')
    fireEvent.focus(button)
    expect(await screen.findByText('Needs admin')).toBeInTheDocument()
  })
})
