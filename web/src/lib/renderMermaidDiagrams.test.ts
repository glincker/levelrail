import { describe, expect, it, vi, beforeEach } from 'vitest'
import { renderMermaidDiagrams } from './renderMermaidDiagrams'

const { initialize, render } = vi.hoisted(() => ({
  initialize: vi.fn(),
  render: vi.fn(),
}))

vi.mock('mermaid', () => ({
  default: { initialize, render },
}))

beforeEach(() => {
  initialize.mockReset()
  render.mockReset()
})

function withPlaceholder(source: string): HTMLElement {
  const container = document.createElement('div')
  const block = document.createElement('div')
  block.dataset.mermaidSource = encodeURIComponent(source)
  block.textContent = source
  container.appendChild(block)
  return container
}

describe('renderMermaidDiagrams', () => {
  it('does nothing when the container has no mermaid placeholders', async () => {
    const container = document.createElement('div')
    await renderMermaidDiagrams(container, false)
    expect(initialize).not.toHaveBeenCalled()
  })

  it('initializes mermaid with the light theme and replaces the placeholder with the rendered svg', async () => {
    render.mockResolvedValue({ svg: '<svg data-testid="diagram"></svg>' })
    const container = withPlaceholder('flowchart TD\n  A --> B')

    await renderMermaidDiagrams(container, false)

    expect(initialize).toHaveBeenCalledWith(
      expect.objectContaining({ theme: 'default', startOnLoad: false }),
    )
    expect(container.innerHTML).toContain('data-testid="diagram"')
    expect(container.innerHTML).not.toContain('flowchart TD')
  })

  it('initializes mermaid with the dark theme when isDark is true', async () => {
    render.mockResolvedValue({ svg: '<svg></svg>' })
    const container = withPlaceholder('flowchart TD\n  A --> B')

    await renderMermaidDiagrams(container, true)

    expect(initialize).toHaveBeenCalledWith(
      expect.objectContaining({ theme: 'dark' }),
    )
  })

  it('leaves the raw-source fallback in place when mermaid fails to render', async () => {
    render.mockRejectedValue(new Error('bad diagram syntax'))
    const container = withPlaceholder('not a real diagram')

    await renderMermaidDiagrams(container, false)

    expect(container.innerHTML).toContain('not a real diagram')
  })
})
