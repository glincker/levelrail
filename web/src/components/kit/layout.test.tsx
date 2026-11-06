import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AllI18n } from '../../test/allI18n'
import { SectionNav, SettingsLayout, SettingsSection } from './SettingsLayout'
import { Tile, TileGrid } from './TileGrid'

type IOCallback = (entries: Partial<IntersectionObserverEntry>[]) => void
let ioCallback: IOCallback | null = null

beforeEach(() => {
  ioCallback = null
  class MockIO {
    constructor(cb: IOCallback) {
      ioCallback = cb
    }
    observe = vi.fn()
    disconnect = vi.fn()
    unobserve = vi.fn()
  }
  vi.stubGlobal('IntersectionObserver', MockIO)
  Element.prototype.scrollIntoView = vi.fn()
})

afterEach(() => vi.unstubAllGlobals())

const wrap = (ui: React.ReactElement) => render(<AllI18n>{ui}</AllI18n>)

describe('TileGrid', () => {
  it('maps sizes to col-span classes', () => {
    wrap(
      <TileGrid>
        <Tile title="a" size="S" />
        <Tile title="b" size="M" />
        <Tile title="c" size="L" rows={2} />
      </TileGrid>,
    )
    const tiles = screen.getAllByTestId('tile')
    const [a, b, c] = tiles as [HTMLElement, HTMLElement, HTMLElement]
    expect(a.className).toContain('xl:col-span-3')
    expect(b.className).toContain('xl:col-span-6')
    expect(c.className).toContain('xl:col-span-12')
    expect(c.className).toContain('row-span-2')
    expect(screen.getByTestId('tile-grid').className).toContain(
      'grid-flow-dense',
    )
  })

  it('hides an empty tile and keeps the others', () => {
    wrap(
      <TileGrid>
        <Tile title="gone" empty hideWhenEmpty />
        <Tile title="kept" empty />
      </TileGrid>,
    )
    expect(screen.queryByText('gone')).toBeNull()
    expect(screen.getByText('kept')).toBeTruthy()
  })

  it('shows a skeleton while loading', () => {
    wrap(
      <Tile title="t" loading>
        body
      </Tile>,
    )
    expect(screen.getByTestId('tile-skeleton')).toBeTruthy()
    expect(screen.queryByText('body')).toBeNull()
  })

  it('shows the error state and calls retry', () => {
    const onRetry = vi.fn()
    wrap(<Tile title="t" error errorLabel="Failed" onRetry={onRetry} />)
    expect(screen.getByRole('alert').textContent).toContain('Failed')
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(onRetry).toHaveBeenCalledOnce()
  })
})

const items = [
  { id: 'one', label: 'One' },
  { id: 'two', label: 'Two' },
]

describe('SectionNav', () => {
  it('marks the active link with aria-current', () => {
    const onNavigate = vi.fn()
    wrap(<SectionNav items={items} active="two" onNavigate={onNavigate} />)
    expect(
      screen.getByRole('link', { name: 'Two' }).getAttribute('aria-current'),
    ).toBe('true')
    expect(
      screen.getByRole('link', { name: 'One' }).getAttribute('aria-current'),
    ).toBeNull()
    fireEvent.click(screen.getByRole('link', { name: 'One' }))
    expect(onNavigate).toHaveBeenCalledWith('one')
  })

  it('navigates from the select fallback', () => {
    const onNavigate = vi.fn()
    wrap(<SectionNav items={items} active="one" onNavigate={onNavigate} />)
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'two' } })
    expect(onNavigate).toHaveBeenCalledWith('two')
  })
})

describe('SectionNav icons', () => {
  it('renders a decorative icon before the label', () => {
    wrap(
      <SectionNav
        items={[{ id: 'one', label: 'One', icon: <svg data-testid="ic" /> }]}
        active="one"
        onNavigate={vi.fn()}
      />,
    )
    const ic = screen.getByTestId('ic')
    expect(ic.parentElement?.getAttribute('aria-hidden')).toBe('true')
  })
})

describe('SettingsLayout', () => {
  it('follows the intersecting section', () => {
    wrap(
      <SettingsLayout sections={items}>
        <SettingsSection id="one" title="One">
          <p>first</p>
        </SettingsSection>
        <SettingsSection id="two" title="Two">
          <p>second</p>
        </SettingsSection>
      </SettingsLayout>,
    )
    const two = document.getElementById('two')
    expect(two).not.toBeNull()
    act(() => ioCallback?.([{ isIntersecting: true, target: two as Element }]))
    expect(
      screen.getByRole('link', { name: 'Two' }).getAttribute('aria-current'),
    ).toBe('true')
  })

  it('clips the shell main only while mounted', () => {
    const main = document.createElement('main')
    document.body.appendChild(main)
    const view = render(
      <AllI18n>
        <SettingsLayout sections={items}>
          <p>x</p>
        </SettingsLayout>
      </AllI18n>,
      { container: main.appendChild(document.createElement('div')) },
    )
    expect(main.classList.contains('overflow-y-clip!')).toBe(true)
    view.unmount()
    expect(main.classList.contains('overflow-y-clip!')).toBe(false)
    main.remove()
  })

  it('renders section children', () => {
    wrap(
      <SettingsSection id="s" title="Sec" description="Desc">
        <p>child</p>
      </SettingsSection>,
    )
    expect(screen.getByRole('heading', { name: 'Sec' })).toBeTruthy()
    expect(screen.getByText('Desc')).toBeTruthy()
    expect(screen.getByText('child')).toBeTruthy()
  })
})
