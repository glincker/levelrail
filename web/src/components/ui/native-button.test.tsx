import { render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { Link } from '@tanstack/react-router'
import { Button } from '@/components/ui/button'
import { Dialog, DialogClose, DialogTrigger } from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Popover, PopoverTrigger } from '@/components/ui/popover'
import { Sheet, SheetTrigger } from '@/components/ui/sheet'
import { Collapsible, CollapsibleTrigger } from '@/components/ui/collapsible'
import { nativeButtonFromRender } from '@/lib/nativeButton'

function baseUiErrors(spy: { mock: { calls: unknown[][] } }): string[] {
  return spy.mock.calls
    .map((c) => String(c[0]))
    .filter((m) => m.includes('nativeButton'))
}

describe('nativeButtonFromRender', () => {
  it('derives the flag from the rendered element', () => {
    expect(nativeButtonFromRender(<a href="/x" />)).toBe(false)
    expect(nativeButtonFromRender(<div />)).toBe(false)
    expect(nativeButtonFromRender(<Link to="/" />)).toBe(false)
    expect(nativeButtonFromRender(<button type="button" />)).toBe(true)
    expect(nativeButtonFromRender(<Button />)).toBeUndefined()
    expect(nativeButtonFromRender(undefined)).toBeUndefined()
    expect(nativeButtonFromRender(<div />, true)).toBe(true)
  })
})

describe('Base UI wrappers with a non-button render', () => {
  afterEach(() => vi.restoreAllMocks())

  it('Button and triggers stay quiet', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    render(
      <>
        <Button render={<a href="/x" />}>Link</Button>
        <Button render={<div />}>Div</Button>
        <Dialog>
          <DialogTrigger render={<div />}>Open</DialogTrigger>
        </Dialog>
        <Sheet>
          <SheetTrigger render={<span />}>Sheet</SheetTrigger>
        </Sheet>
        <Popover>
          <PopoverTrigger render={<div />}>Pop</PopoverTrigger>
        </Popover>
        <DropdownMenu>
          <DropdownMenuTrigger render={<div />}>Menu</DropdownMenuTrigger>
        </DropdownMenu>
        <Collapsible>
          <CollapsibleTrigger render={<div />}>Fold</CollapsibleTrigger>
        </Collapsible>
        <Dialog open>
          <DialogClose render={<a href="/y" />}>Close</DialogClose>
        </Dialog>
      </>,
    )
    expect(baseUiErrors(spy)).toEqual([])
  })

  it('a real button render stays native and quiet', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    render(
      <Dialog>
        <DialogTrigger render={<Button />}>Open</DialogTrigger>
      </Dialog>,
    )
    expect(baseUiErrors(spy)).toEqual([])
  })
})
