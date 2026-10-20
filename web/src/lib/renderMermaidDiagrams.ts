const MERMAID_SELECTOR = '[data-mermaid-source]'

let nextDiagramId = 0

// Hydrates every `renderDocsMarkdown`-emitted mermaid placeholder inside
// `container` into a rendered SVG, in place. Dynamically imported so
// mermaid never lands in the main bundle: most doc pages have no
// diagrams at all. Errors from a single diagram (bad syntax) are
// swallowed so the rest of the page, and the raw-source fallback already
// in the DOM, keep working.
//
// isCurrent lets a caller that can fire this again before the previous
// call finishes (DocsRenderer re-runs it on every theme toggle) bail out
// of a now-superseded run instead of racing it: mermaid.initialize sets
// shared, module-global theme state, so two overlapping calls with
// different themes can otherwise interleave and leave some diagrams
// rendered in the wrong one.
export async function renderMermaidDiagrams(
  container: HTMLElement,
  isDark: boolean,
  isCurrent: () => boolean = () => true,
): Promise<void> {
  const blocks = container.querySelectorAll<HTMLElement>(MERMAID_SELECTOR)
  if (blocks.length === 0) return

  const { default: mermaid } = await import('mermaid')
  if (!isCurrent()) return
  mermaid.initialize({
    startOnLoad: false,
    securityLevel: 'strict',
    theme: isDark ? 'dark' : 'default',
  })

  for (const block of blocks) {
    if (!isCurrent()) return
    const encoded = block.dataset.mermaidSource
    if (!encoded) continue
    const source = decodeURIComponent(encoded)
    try {
      const { svg } = await mermaid.render(
        `mermaid-diagram-${nextDiagramId++}`,
        source,
      )
      if (!isCurrent()) return
      block.innerHTML = svg
    } catch {
      // Leave the raw-source fallback markup already rendered in place.
    }
  }
}
