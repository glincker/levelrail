import { afterEach, describe, expect, it, vi } from 'vitest'
import { scrollActiveNavItemIntoView } from './scrollActiveNavItem'

function rect(top: number, bottom: number): DOMRect {
  return {
    top,
    bottom,
    left: 0,
    right: 0,
    width: 0,
    height: bottom - top,
  } as DOMRect
}

function mount(rowTop: number, rowBottom: number) {
  document.body.innerHTML = `
    <div data-slot="sidebar-content">
      <a data-sidebar="menu-button" data-active="" aria-current="page">Traffic</a>
    </div>`
  const scroller = document.querySelector<HTMLElement>(
    '[data-slot="sidebar-content"]',
  )!
  const row = document.querySelector<HTMLElement>('[data-active]')!
  scroller.getBoundingClientRect = () => rect(100, 300)
  row.getBoundingClientRect = () => rect(rowTop, rowBottom)
  scroller.scrollTop = 50
  const scrollTo = vi.fn()
  scroller.scrollTo = scrollTo
  return scrollTo
}

describe('scrollActiveNavItemIntoView', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('does nothing when the active row is fully visible', () => {
    const scrollTo = mount(150, 180)
    scrollActiveNavItemIntoView()
    expect(scrollTo).not.toHaveBeenCalled()
  })

  it('scrolls down instantly just enough when the row is below the fold', () => {
    const scrollTo = mount(280, 330)
    scrollActiveNavItemIntoView()
    expect(scrollTo).toHaveBeenCalledWith({ top: 80, behavior: 'instant' })
  })

  it('scrolls up just enough when the row is above the fold', () => {
    const scrollTo = mount(70, 110)
    scrollActiveNavItemIntoView()
    expect(scrollTo).toHaveBeenCalledWith({ top: 20, behavior: 'instant' })
  })

  it('ignores a page with no active row', () => {
    document.body.innerHTML = '<div data-slot="sidebar-content"></div>'
    expect(() => {
      scrollActiveNavItemIntoView()
    }).not.toThrow()
  })
})
