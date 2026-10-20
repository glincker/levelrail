import { render, waitFor } from '@testing-library/react'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { DocsRenderer } from './DocsRenderer'
import type { DocsManifest } from '../types/docs'

const navigate = vi.fn()
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigate,
}))

const renderMermaidDiagrams = vi.hoisted(() =>
  vi.fn<
    (
      container: HTMLElement,
      isDark: boolean,
      isCurrent: () => boolean,
    ) => Promise<void>
  >(async () => {}),
)
vi.mock('../lib/renderMermaidDiagrams', () => ({ renderMermaidDiagrams }))

const manifest: DocsManifest = { categories: [], pages: {} }

beforeEach(() => {
  renderMermaidDiagrams.mockClear()
  document.documentElement.classList.remove('dark')
})

describe('DocsRenderer', () => {
  it('hydrates mermaid diagrams against the rendered container after mount', async () => {
    render(
      <DocsRenderer
        markdown={'```mermaid\nflowchart TD\n  A --> B\n```\n'}
        currentFile="architecture.md"
        manifest={manifest}
        docsBaseUrl=""
      />,
    )

    await waitFor(() => {
      expect(renderMermaidDiagrams).toHaveBeenCalledTimes(1)
    })
    const call = renderMermaidDiagrams.mock.calls[0]
    if (!call)
      throw new Error('expected renderMermaidDiagrams to have been called')
    const [container, isDark] = call
    expect(container).toBeInstanceOf(HTMLElement)
    expect(isDark).toBe(false)
  })

  it('passes isDark true when the document is in dark mode', async () => {
    document.documentElement.classList.add('dark')

    render(
      <DocsRenderer
        markdown={'# Title'}
        currentFile="x.md"
        manifest={manifest}
        docsBaseUrl=""
      />,
    )

    await waitFor(() => {
      expect(renderMermaidDiagrams).toHaveBeenCalledWith(
        expect.anything(),
        true,
        expect.any(Function),
      )
    })
  })

  it('a second theme toggle marks the first call stale, so a slow first render cannot win', async () => {
    render(
      <DocsRenderer
        markdown={'```mermaid\nflowchart TD\n  A --> B\n```\n'}
        currentFile="architecture.md"
        manifest={manifest}
        docsBaseUrl=""
      />,
    )
    await waitFor(() => {
      expect(renderMermaidDiagrams).toHaveBeenCalledTimes(1)
    })
    const [, , firstIsCurrent] = renderMermaidDiagrams.mock.calls[0] ?? []
    if (!firstIsCurrent) throw new Error('expected an isCurrent function')

    document.documentElement.classList.add('dark')
    await waitFor(() => {
      expect(renderMermaidDiagrams).toHaveBeenCalledTimes(2)
    })
    const [, , secondIsCurrent] = renderMermaidDiagrams.mock.calls[1] ?? []
    if (!secondIsCurrent) throw new Error('expected an isCurrent function')

    expect(firstIsCurrent()).toBe(false)
    expect(secondIsCurrent()).toBe(true)
  })
})
