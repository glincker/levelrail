const MERMAID_SELECTOR = '[data-mermaid-source]'

let nextDiagramId = 0

// Hydrates every `renderDocsMarkdown`-emitted mermaid placeholder inside
// `container` into a rendered SVG, in place. Dynamically imported so
// mermaid never lands in the main bundle: most doc pages have no
// diagrams at all. Errors from a single diagram (bad syntax) are
// swallowed so the rest of the page, and the raw-source fallback already
// in the DOM, keep working.
export async function renderMermaidDiagrams(
  container: HTMLElement,
  isDark: boolean,
): Promise<void> {
  const blocks = container.querySelectorAll<HTMLElement>(MERMAID_SELECTOR)
  if (blocks.length === 0) return

  const { default: mermaid } = await import('mermaid')
  mermaid.initialize({
    startOnLoad: false,
    securityLevel: 'strict',
    theme: isDark ? 'dark' : 'default',
  })

  for (const block of blocks) {
    const encoded = block.dataset.mermaidSource
    if (!encoded) continue
    const source = decodeURIComponent(encoded)
    try {
      const { svg } = await mermaid.render(
        `mermaid-diagram-${nextDiagramId++}`,
        source,
      )
      block.innerHTML = svg
    } catch {
      // Leave the raw-source fallback markup already rendered in place.
    }
  }
}
