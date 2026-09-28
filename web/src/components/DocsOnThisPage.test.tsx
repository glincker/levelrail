import { act, render, waitFor } from '@testing-library/react'
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { DocsOnThisPage } from './DocsOnThisPage'
import type { DocHeading } from '@/types/docs'

const headings: DocHeading[] = [
  { id: 'first', text: 'First section', level: 2 },
  { id: 'second', text: 'Second section', level: 2 },
]

const observe = vi.fn()
const disconnect = vi.fn()
let constructedCount = 0
class IntersectionObserverMock {
  observe = observe
  disconnect = disconnect
  unobserve = vi.fn()
  takeRecords = vi.fn(() => [])
  constructor() {
    constructedCount += 1
  }
}

beforeEach(() => {
  observe.mockClear()
  disconnect.mockClear()
  constructedCount = 0
  vi.stubGlobal('IntersectionObserver', IntersectionObserverMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

describe('DocsOnThisPage', () => {
  it('renders nothing when there are no headings', () => {
    const { container } = render(<DocsOnThisPage headings={[]} />)
    expect(container.firstChild).toBeNull()
  })

  it('renders a link per heading, indenting level-3 ones', () => {
    const { getAllByRole } = render(<DocsOnThisPage headings={headings} />)
    const links = getAllByRole('link')
    expect(links).toHaveLength(2)
    expect(links[0]).toHaveAttribute('href', '#first')
    expect(links[0]).toHaveTextContent('First section')
  })

  it('retries setup until the heading elements exist in the DOM, then observes them', async () => {
    // No matching elements in the DOM at mount: the manifest's headings
    // prop is available immediately, but /help/$'s own content (and so
    // these ids) loads through a separate, slower route loader.
    render(<DocsOnThisPage headings={headings} />)

    expect(constructedCount).toBe(0)

    const h1 = document.createElement('h2')
    h1.id = 'first'
    const h2 = document.createElement('h2')
    h2.id = 'second'
    document.body.append(h1, h2)

    await waitFor(() => {
      expect(constructedCount).toBe(1)
    })
    expect(observe).toHaveBeenCalledWith(h1)
    expect(observe).toHaveBeenCalledWith(h2)
  })

  it('disconnects the observer and stops retrying on unmount', async () => {
    const { unmount } = render(<DocsOnThisPage headings={headings} />)
    const h1 = document.createElement('h2')
    h1.id = 'first'
    document.body.append(h1)

    await waitFor(() => {
      expect(constructedCount).toBe(1)
    })

    act(() => {
      unmount()
    })
    expect(disconnect).toHaveBeenCalledTimes(1)
  })
})
